package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// 本文件把「发信失败」翻译成运维能照着做的中文提示，并统一两条测试发信路径
// （后台邮件设置 SendTest、安装向导 SendSetupTestMail）的行为。
//
// 要解决的问题（真实工单）：
//
//	测试邮件发送失败：[10007] 服务器内部错误: SMTP 认证失败: 535 "Login fail. ..."
//
// 这句话有两处误导：535 是邮箱服务商对「授权码填成了登录密码 / POP3-SMTP 服务没开 /
// 账号只填了前缀」的统一回应，属于**使用者配置问题**，套上「服务器内部错误」会让运维
// 去翻代码和日志，而真正要改的是邮箱网页端的一个开关；同时它也没告诉运维该改什么。
//
// 因此这里的产出是「专用错误码 10009 + 排查步骤 + 服务商专属提示 + 服务器原文」，
// 并且**任何分支都不再拼接 "[10007] 服务器内部错误"**。

// smtpAuthSteps 是认证失败的通用排查步骤（不区分服务商的四条）。
//
// 刻意写成「按顺序检查」：四条的命中率从高到低——第 1 条（授权码 vs 登录密码）
// 覆盖了绝大多数 535；第 4 条是唯一与参数相关的一条。
const smtpAuthSteps = `请按以下顺序检查
1) 密码是否是邮箱服务商生成的「授权码」——QQ/163/腾讯企业邮都必须用授权码，不能用登录密码；
2) 该账号是否已在邮箱网页端开启 SMTP/POP3 服务；
3) 账号是否填写为完整邮箱地址（如 xxx@qq.com），不要只填前缀；
4) 服务器与端口是否匹配（个人 QQ 邮箱 smtp.qq.com:465，腾讯企业邮 smtp.exmail.qq.com:465）。`

// smtpHostHint 返回针对某家邮箱服务商的补充建议；不在内置表里时返回空串。
//
// 判定规则只有一条：主机名转小写、去首尾空白后做**包含**匹配。
// 不用等值匹配是因为运维常填 `smtp.qq.com`、也常填带端口或别名的写法，
// 包含匹配对拼写变体更宽容。
//
// 顺序不能随意调整：`smtp.exmail.qq.com` 同时包含 `qq.com` 与 `exmail.qq.com`，
// 因此企业邮这一支必须排在 QQ 邮箱之前，否则企业邮用户会拿到个人邮的指引
// （两者的授权码不通用，这是实际会踩的坑）。
func smtpHostHint(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	switch {
	case h == "":
		return ""
	case strings.Contains(h, "exmail.qq.com"):
		return "你填的是腾讯企业邮（smtp.exmail.qq.com）：请用管理员登录企业邮管理端，在「邮箱设置 → 客户端设置」开启 IMAP/SMTP 服务，并生成「客户端专用密码」填到这里。注意它与个人 QQ 邮箱（smtp.qq.com）的授权码互不通用。"
	case strings.Contains(h, "qq.com"):
		return "你填的是 QQ 邮箱（smtp.qq.com）：请到 QQ 邮箱网页端「设置 → 账户 → POP3/IMAP/SMTP 服务」开启服务，按提示用手机发送短信后获取 16 位「授权码」——登录密码一定不行，且授权码只在生成时显示一次。"
	case strings.Contains(h, "163.com") || strings.Contains(h, "126.com"):
		return "你填的是网易邮箱（163/126）：请到邮箱网页端「设置 → POP3/SMTP/IMAP」开启 SMTP 服务，并把开启时设置的「客户端授权密码」当作这里的密码（与网页登录密码不同）。"
	case strings.Contains(h, "gmail.com"):
		return "你填的是 Gmail（smtp.gmail.com）：需先在 Google 账号中开启两步验证，再生成 16 位「应用专用密码」；若你的组织策略禁止应用专用密码，请改用 OAuth（XOAUTH2）。"
	case strings.Contains(h, "office365.com") || strings.Contains(h, "outlook.com"):
		return "你填的是 Microsoft 365 / Outlook（smtp.office365.com）：用户名必须是完整邮箱地址；若租户开启了安全默认值或多因素认证，SMTP AUTH 可能已被禁用，需要在租户中显式启用 SMTP AUTH，或使用应用密码。"
	default:
		return ""
	}
}

// smtpAuthFailureMessage 组装认证失败的完整对外文案。
//
// code 为 0 时不写「服务器返回 x」；raw 为空时不写「服务器原文」——
// 实际链路里两者几乎总有值，保留分支只是为了让函数在任何输入下都产出可读文本。
func smtpAuthFailureMessage(host string, code int, raw string) string {
	var sb strings.Builder
	if code > 0 {
		fmt.Fprintf(&sb, "SMTP 认证失败（服务器返回 %d）：%s", code, smtpAuthSteps)
	} else {
		sb.WriteString("SMTP 认证失败：" + smtpAuthSteps)
	}
	if hint := smtpHostHint(host); hint != "" {
		sb.WriteString("\n5) " + hint)
	}
	if raw = strings.TrimSpace(raw); raw != "" {
		// 服务器原文必须原样保留：运维要拿它去搜服务商文档或开工单，
		// 我们改写或截断它只会让「对不上号」。
		sb.WriteString("\n服务器原文：" + raw)
	}
	return sb.String()
}

// tlsMismatchSteps 是「端口与加密方式不匹配」的三条对照关系。
const tlsMismatchSteps = `1) 465 端口必须选「SSL/TLS 直连」（隐式 TLS）；
2) 587 端口必须选「STARTTLS」（不要选 SSL/TLS 直连）；
3) 25 端口通常被云厂商与运营商封禁，建议改用 465 或 587。`

// encryptionLabel 把「当前提交的加密方式」翻译成安装向导/后台里的选项名。
//
// 后端只有一个 tls 布尔值，而界面上有三档（SSL/TLS 直连 465、STARTTLS 587、不加密 25）。
// 只报 tls=false 运维对不上号，因此按端口还原成他当时点的那一档。
func encryptionLabel(port int, tls bool) string {
	switch {
	case tls:
		return "SSL/TLS 直连（tls=true）"
	case port == 587:
		return "STARTTLS（tls=false）"
	case port == 25:
		return "不加密（tls=false）"
	default:
		return "STARTTLS 或不加密（tls=false）"
	}
}

// tlsMismatchAdvice 针对「当前端口 + 当前加密方式」给出那一句诊断。
//
// 这是整条提示里最能省时间的一句：它把「EOF」直接翻译成
// 「465 用明文连接，服务器等不到 TLS 客户端问候就把连接关了」。
func tlsMismatchAdvice(port int, tls bool) string {
	switch {
	case !tls && port == 465:
		return "该组合下服务器会因收到明文而立即断开，请在加密方式里改选「SSL/TLS 直连」。"
	case tls && port == 587:
		return "587 端口只接受先明文再升级 TLS，请把加密方式改选「STARTTLS」（tls=false）。"
	case tls && port == 25:
		return "25 端口通常不通告 STARTTLS，且多数云厂商已封禁，建议改用 465 +「SSL/TLS 直连」。"
	case !tls && port == 25:
		return "25 端口多被云厂商封禁；若服务商要求加密，请改用 465（SSL/TLS 直连）或 587（STARTTLS）。"
	default:
		return "请对照服务商的「客户端 SMTP 设置」核对端口与加密方式（465↔SSL/TLS 直连，587↔STARTTLS）。"
	}
}

// smtpConnectFailureMessage 组装连接阶段失败的完整对外文案。
//
// 结构固定为三段：结论 + 可照做的动作、当前提交的参数、服务器原文。
// 第二段是关键：把 host/port/加密方式回显出来，运维才能一眼锁定「我选错了哪一项」。
// 口令绝不出现（连接阶段拿不到口令，也不会被写进任何一句话）。
func smtpConnectFailureMessage(c *mail.ConnectError) string {
	raw := strings.TrimSpace(c.Raw)
	if raw == "" {
		raw = "对端未返回任何内容"
	}

	var head string
	switch c.Kind {
	case mail.ConnectTLSMismatch:
		head = fmt.Sprintf("无法建立 SMTP 连接（%s）：端口与加密方式很可能不匹配\n%s", raw, tlsMismatchSteps)
	case mail.ConnectHostUnresolved:
		head = fmt.Sprintf("无法解析 SMTP 主机名 %s：请检查主机名是否拼写正确（不要带协议前缀与端口）", hostOrPlaceholder(c.Host))
	case mail.ConnectRefused:
		head = "SMTP 服务器拒绝连接：请检查端口（465/587）以及服务商是否已为该账号开启 SMTP 服务"
	case mail.ConnectTimeout:
		head = "连接 SMTP 服务器超时：请检查网络/安全组是否放行出站 465/587（部分云主机默认封禁 25 端口）"
	default:
		head = "无法连接 SMTP 服务器：" + raw
	}

	var sb strings.Builder
	sb.WriteString(head)
	fmt.Fprintf(&sb, "\n当前提交：%s，加密方式=%s", connectTarget(c), encryptionLabel(c.Port, c.TLS))
	if c.Kind == mail.ConnectTLSMismatch {
		sb.WriteString("—— " + tlsMismatchAdvice(c.Port, c.TLS))
	}
	sb.WriteString("\n服务器原文：" + raw)
	return sb.String()
}

// connectTarget 返回 "host:port" 形式的目标；host 缺失时给出占位提示。
func connectTarget(c *mail.ConnectError) string {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		host = "（主机名为空，请填写 SMTP 服务器地址）"
	}
	if c.Port <= 0 {
		return host
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
}

// hostOrPlaceholder 在主机名为空时给出可读占位，避免出现「主机名 ：」这种空指代。
func hostOrPlaceholder(host string) string {
	if h := strings.TrimSpace(host); h != "" {
		return h
	}
	return "（未填写）"
}

// describeMailFailure 把投递错误翻译成业务错误。
//
// 四条分支：
//  1. 认证失败 → 10009（HTTP 503）+ 排查步骤 + 服务商提示 + 服务器原文；
//  2. 连接/握手失败 → 10008（HTTP 503）+ 归因提示 + 当前提交参数 + 服务器原文；
//  3. 其它（MAIL FROM / RCPT / DATA 被拒等）→ 10008 + 原样原因。
//
// 所有分支都保证 message 里不出现「服务器内部错误」：那是对**我们**的诊断，
// 对运维是无信息的噪声，还会把「配置问题」误导成「程序坏了」。
func describeMailFailure(action string, err error) *apperr.Error {
	var authErr *mail.AuthError
	if errors.As(err, &authErr) {
		return apperr.ErrMailAuthFailed.WithMessage(
			smtpAuthFailureMessage(authErr.Host, authErr.Code, authErr.Raw))
	}

	var connErr *mail.ConnectError
	if errors.As(err, &connErr) {
		return apperr.ErrUnavailable.WithMessage(smtpConnectFailureMessage(connErr))
	}

	reason := plainFailureReason(err)
	if reason == "" {
		reason = "原因未知，请查看服务端日志"
	}
	return apperr.ErrUnavailable.WithMessage(action + "：" + reason)
}

// plainFailureReason 取出错误的可读原因，并剥掉底层的「[10007] 服务器内部错误」前缀。
//
// 为什么需要剥：internal/mail 之外的代码（以及历史代码）可能用
// apperr.ErrInternal.WithCause(err) 包一层，其 Error() 是
// `[10007] 服务器内部错误: <真实原因>`；直接拼给运维就又回到了工单里的那句误导文案。
func plainFailureReason(err error) string {
	if err == nil {
		return ""
	}
	if biz, ok := apperr.As(err); ok && biz.Code == apperr.CodeInternal {
		if cause := errors.Unwrap(biz); cause != nil {
			return cause.Error()
		}
	}
	return err.Error()
}

// trimMailSecret 去除 SMTP 密码/授权码的首尾空白，并在确实发生变化时记一条告警。
//
// 为什么是「自动去除」而不是「拒绝保存」：
//   - 首尾空白几乎总是复制授权码时的产物（网页端常带一个尾随空格或换行），
//     它本身不改变口令语义，没有任何理由让运维为此重填一次；
//   - 若直接拒绝，安装向导能过、后台设置那条路径却可能没做同样检查，
//     两条路径的行为会不一致（本次修复前的真实状况）。
//
// 日志只记录「发生了变化」这一事实与主机、账号（可公开字段），**绝不输出口令本身**。
func trimMailSecret(password, host, username string) string {
	trimmed := strings.TrimSpace(password)
	if trimmed != password {
		slog.Warn("检测到 SMTP 密码/授权码首尾含空白字符，已自动去除",
			"host", host,
			"username", username,
		)
	}
	return trimmed
}

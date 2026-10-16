package mail

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/textproto"
	"strconv"
	"strings"
)

// 本文件把 SMTP 的失败分成两类，并**原样保留服务器响应**：
//
//   - 认证失败（*AuthError）：535/534/530/454，或错误文本里带 auth / login fail 等特征。
//     这类失败的根因几乎总在**使用者配置**——授权码填成了登录密码、POP3/SMTP 服务没开、
//     账号只填了前缀、端口与加密方式不匹配。它需要一条能照着做的中文提示，
//     而不是一句「服务器内部错误」把排查方向带偏。
//   - 连接失败（*ConnectError）：DNS 解析不了、端口拒绝、TLS 握手失败。
//     这类失败要直说「连不上」，并保留底层原文（`connection refused` / `i/o timeout`），
//     让运维一眼看出是网络还是参数问题。
//
// 两类都实现 Is/Unwrap，因此调用方既能用 errors.Is(err, ErrAuthFailed) 做判定，
// 也能用 errors.As 取出 Code/Raw 生成文案。

// redactMask 是口令的替换掩码。
const redactMask = "****"

// ErrAuthFailed 是「SMTP 认证被服务器拒绝」的哨兵错误，用 errors.Is 判定。
var ErrAuthFailed = errors.New("SMTP 认证失败")

// ErrConnectFailed 是「无法与 SMTP 服务器建立连接」的哨兵错误。
var ErrConnectFailed = errors.New("无法连接 SMTP 服务器")

// ErrTLSMismatch 是「端口与加密方式不匹配」的哨兵错误。
//
// 触发它的典型现象：把 465 端口选成 STARTTLS/不加密（或反过来），
// 对端在收到明文/TLS 客户端问候时直接断开，表现为 EOF、
// `tls: first record does not look like a TLS handshake` 或 connection reset。
var ErrTLSMismatch = errors.New("端口与加密方式不匹配")

// ErrHostUnresolved 是「主机名解析失败」的哨兵错误。
var ErrHostUnresolved = errors.New("无法解析 SMTP 主机名")

// ErrConnectRefused 是「端口无人监听 / 服务未开启」的哨兵错误。
var ErrConnectRefused = errors.New("SMTP 服务器拒绝连接")

// ErrConnectTimeout 是「连接超时」的哨兵错误。
var ErrConnectTimeout = errors.New("连接 SMTP 服务器超时")

// AuthError 描述一次认证失败。
//
// 字段全部导出：调用方（service 层）要拿 Host 生成服务商专属提示、
// 拿 Code 写进文案、拿 Raw 把服务器原话展示给运维。
type AuthError struct {
	// Host 是本次连接使用的 SMTP 主机名，用于生成「QQ 邮箱要这样配」这类提示。
	Host string
	// Code 是 SMTP 响应码（如 535）；无法从错误里解析出响应码时为 0。
	Code int
	// Raw 是服务器原始响应文本（Go 的 textproto 会格式化为 `535 "Login fail. ..."`）。
	// **不加工、不截断**：运维要拿它去搜索引擎或服务商工单里对照。
	Raw string

	cause error
}

// Error 实现 error 接口。
func (e *AuthError) Error() string {
	if e.Code > 0 {
		return fmt.Sprintf("SMTP 认证失败（服务器返回 %d）：%s", e.Code, e.Raw)
	}
	return "SMTP 认证失败：" + e.Raw
}

// Is 让 errors.Is(err, ErrAuthFailed) 成立。
//
// 这里不用「Unwrap 到某个哨兵」的写法：本类型还要保留底层错误链
// （Unwrap 返回 cause），而 errors.Is 只走一条链，因此显式实现 Is。
func (e *AuthError) Is(target error) bool { return target == ErrAuthFailed }

// Unwrap 保留底层错误链，便于日志里看到最原始的原因。
func (e *AuthError) Unwrap() error { return e.cause }

// ConnectKind 是连接失败的细分归因，决定服务层给出哪一条可照做的提示。
type ConnectKind string

const (
	// ConnectUnknown 表示无法归因（罕见：协议实现异常等）。
	ConnectUnknown ConnectKind = ""
	// ConnectTLSMismatch：对端在 TLS 握手前后直接断开，或 TLS 协议错配。
	// 最常见的原因就是「465 端口选了 STARTTLS / 不加密」。
	ConnectTLSMismatch ConnectKind = "tls_mismatch"
	// ConnectHostUnresolved：DNS 解析失败，主机名拼错。
	ConnectHostUnresolved ConnectKind = "host_unresolved"
	// ConnectRefused：端口没有服务在监听，或服务商关闭了 SMTP 服务。
	ConnectRefused ConnectKind = "connection_refused"
	// ConnectTimeout：网络不通，或被安全组/防火墙拦截（云主机常封 25 端口）。
	ConnectTimeout ConnectKind = "timeout"
)

// ConnectError 描述一次连接/握手失败。
//
// 之所以要带上 Host/Port/TLS：给运维的提示里要回显「当前提交：host:port，加密方式=X」，
// 让他一眼看出自己选错了哪一项，而不是只看一句 EOF。口令绝不进入本类型。
type ConnectError struct {
	// Kind 是归因结果。
	Kind ConnectKind
	// Host / Port / TLS 是本次实际提交的连接参数。
	Host string
	Port int
	TLS  bool
	// Stage 说明失败发生在哪一步；为空表示「建立 TCP 连接并读取问候语」。
	Stage string
	// Raw 是底层原因原文，例如 `EOF`、`dial tcp 1.2.3.4:465: i/o timeout`。
	Raw string

	cause error
}

// Error 实现 error 接口。
func (e *ConnectError) Error() string {
	target := e.target()
	switch {
	case target != "" && strings.TrimSpace(e.Stage) != "":
		return fmt.Sprintf("%s（%s，%s阶段）：%s", ErrConnectFailed.Error(), target, e.Stage, e.Raw)
	case target != "":
		return fmt.Sprintf("%s（%s）：%s", ErrConnectFailed.Error(), target, e.Raw)
	default:
		return ErrConnectFailed.Error() + "：" + e.Raw
	}
}

// target 返回 "host:port" 形式的目标描述（host 为空时返回空串）。
func (e *ConnectError) target() string {
	if strings.TrimSpace(e.Host) == "" {
		return ""
	}
	if e.Port <= 0 {
		return e.Host
	}
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// Is 让 errors.Is 同时支持「广义连接失败」与「细分归因」两种问法：
//
//	errors.Is(err, ErrConnectFailed) // 只要连不上就为真
//	errors.Is(err, ErrTLSMismatch)   // 只有端口/加密方式错配才为真
func (e *ConnectError) Is(target error) bool {
	if target == ErrConnectFailed {
		return true
	}
	return target != nil && target == sentinelOfKind(e.Kind)
}

// sentinelOfKind 返回归因对应的哨兵错误，供 errors.Is 比较。
func sentinelOfKind(kind ConnectKind) error {
	switch kind {
	case ConnectTLSMismatch:
		return ErrTLSMismatch
	case ConnectHostUnresolved:
		return ErrHostUnresolved
	case ConnectRefused:
		return ErrConnectRefused
	case ConnectTimeout:
		return ErrConnectTimeout
	default:
		return nil
	}
}

// Unwrap 保留底层错误链。
func (e *ConnectError) Unwrap() error { return e.cause }

// connectErrorHints 把底层错误文本映射到归因，顺序即优先级（先匹配者生效）。
//
// 为什么用文本特征而不是错误类型：连接失败在 Go 里几乎都是
// *net.OpError / *os.SyscallError / *tls.* 的包装，逐一类型断言既脆弱又
// 覆盖不全；而 net 包的错误文本相当稳定，且这些特征词本身就足以定性。
var connectErrorHints = []struct {
	kind  ConnectKind
	hints []string
}{
	// EOF / 连接被重置 / broken pipe：对端在协议握手阶段直接断开。
	// 实例：465 端口用明文连接（服务器等 ClientHello，等不到就关连接）→ EOF。
	{ConnectTLSMismatch, []string{
		"eof",
		"first record does not look like a tls handshake",
		"connection reset by peer",
		"broken pipe",
		"tls: ",
	}},
	{ConnectHostUnresolved, []string{"no such host"}},
	{ConnectRefused, []string{"connection refused"}},
	{ConnectTimeout, []string{"i/o timeout", "deadline exceeded", "timeout"}},
}

// newConnectError 归因连接失败；err 为 nil 时返回 nil。
func newConnectError(err error, stage, host string, port int, tls bool) *ConnectError {
	if err == nil {
		return nil
	}
	return &ConnectError{
		Kind:  classifyConnectKind(err),
		Host:  host,
		Port:  port,
		TLS:   tls,
		Stage: stage,
		Raw:   err.Error(),
		cause: err,
	}
}

// classifyConnectKind 按错误文本归因；无法归因时返回 ConnectUnknown。
func classifyConnectKind(err error) ConnectKind {
	if err == nil {
		return ConnectUnknown
	}
	text := strings.ToLower(err.Error())
	for _, rule := range connectErrorHints {
		for _, hint := range rule.hints {
			if strings.Contains(text, hint) {
				return rule.kind
			}
		}
	}
	return ConnectUnknown
}

// authErrorCodes 是 SMTP 对「认证/凭据」问题的响应码。
//
//	535 认证失败（QQ / 163 / 腾讯企业邮对「授权码不对或服务没开」的统一回应）
//	534 需要更强的认证机制（常见于要求「应用专用密码」的服务商）
//	530 需要先认证（部分服务器对未认证会话的回应）
//	454 临时认证失败（频率限制、或要求应用专用密码）
var authErrorCodes = map[int]bool{535: true, 534: true, 530: true, 454: true}

// authErrorHints 是错误文本特征，全部按小写比较。
//
// 为什么要文本兜底：不同实现的错误类型不一定是 *textproto.Error
// （可能被中间件包成自定义错误、或被网关改写了响应码），而这类
// 失败的真实原因与 535 完全一致，同样需要那份排查步骤。
var authErrorHints = []string{"auth", "login fail", "login failed", "authentication", "认证失败"}

// classifyAuthFailure 判定错误是否为认证失败；是则返回带服务器原文的 *AuthError，否则返回 nil。
//
// username / password 只用于**脱敏**，不会出现在返回的错误里。
func classifyAuthFailure(host, username, password string, err error) *AuthError {
	if err == nil {
		return nil
	}

	var existing *AuthError
	if errors.As(err, &existing) {
		return existing
	}

	raw := redactAuthError(err.Error(), username, password)

	// 1) 权威信号：SMTP 响应码。
	var proto *textproto.Error
	if errors.As(err, &proto) && authErrorCodes[proto.Code] {
		return &AuthError{Host: host, Code: proto.Code, Raw: raw, cause: err}
	}

	// 2) 文本特征：兜住响应码被改写或错误被包装的情况。
	code := authCodeInText(raw)
	lower := strings.ToLower(raw)
	matched := false
	for _, hint := range authErrorHints {
		if strings.Contains(lower, hint) {
			matched = true
			break
		}
	}
	if !matched && code == 0 {
		return nil
	}
	return &AuthError{Host: host, Code: code, Raw: raw, cause: err}
}

// authCodeInText 从任意文本里提取属于 authErrorCodes 的响应码；没有则返回 0。
//
// 用「数字串精确匹配」而不是 strings.Contains("535")：后者会把
// 端口号、IP、消息 ID 里的 535 误判成响应码。
func authCodeInText(text string) int {
	for _, field := range strings.FieldsFunc(text, func(r rune) bool { return r < '0' || r > '9' }) {
		if len(field) != 3 {
			continue
		}
		n, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		if authErrorCodes[n] {
			return n
		}
	}
	return 0
}

// redactSecrets 把文本中可能出现的口令替换为掩码。
//
// 多数服务器不会回显凭据，但个别中继/网关会把整行 AUTH 请求写进错误文本里，
// 而「任何日志、审计、接口响应都不得出现口令明文」是硬性要求，因此这里做一次兜底：
// 明文与其 AUTH PLAIN 的 base64 形式（base64("\x00user\x00pass")）都会被替换。
func redactSecrets(text, secret string) string {
	if secret == "" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, redactMask)
}

// redactAuthError 在 redactSecrets 的基础上额外覆盖 AUTH PLAIN 的 base64 凭据。
//
// 之所以需要 username 才能算 base64：PLAIN 机制的凭据是 "\x00用户名\x00口令"。
func redactAuthError(text, username, password string) string {
	out := redactSecrets(text, password)
	if password == "" || out == "" {
		return out
	}
	blob := base64.StdEncoding.EncodeToString([]byte("\x00" + username + "\x00" + password))
	if blob == "" {
		return out
	}
	return strings.ReplaceAll(out, blob, redactMask)
}

// redactedError 是一个「Error() 已脱敏但仍可 Unwrap」的包装。
//
// 直接 fmt.Errorf("%w") 会把底层原文原样带出去，若原文里恰好含口令就泄漏了；
// 丢掉 %w 又会切断错误链。这个类型两头都保住。
type redactedError struct {
	msg   string
	cause error
}

// Error 实现 error 接口。
func (e *redactedError) Error() string { return e.msg }

// Unwrap 保留底层错误链。
func (e *redactedError) Unwrap() error { return e.cause }

// newRedactedError 生成脱敏后的包装错误。
func newRedactedError(prefix, username, password string, err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{
		msg:   prefix + ": " + redactAuthError(err.Error(), username, password),
		cause: err,
	}
}

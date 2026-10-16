// Package apperr 定义统一的业务错误码与错误类型。
//
// 错误码分段约定：
//
//	0      成功
//	1xxxx  通用（参数、鉴权、限流、服务端）
//	2xxxx  认证与账号
//	3xxxx  公司 / 岗位 / 审核
//	4xxxx  爬虫
//	5xxxx  系统配置
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Code 是业务错误码。
type Code int

// 通用错误码。
const (
	CodeSuccess       Code = 0
	CodeUnknown       Code = 10000
	CodeInvalidParams Code = 10001
	CodeUnauthorized  Code = 10002
	CodeForbidden     Code = 10003
	CodeNotFound      Code = 10004
	CodeConflict      Code = 10005
	CodeTooManyReqs   Code = 10006
	CodeInternal      Code = 10007
	CodeUnavailable   Code = 10008
	// CodeMailAuthFailed 是「SMTP 认证被服务器拒绝」的专用码。
	//
	// 为什么不能复用 CodeUnavailable：认证失败的根因在**使用者配置**
	// （授权码填成了登录密码、POP3/SMTP 服务未开启、账号只填了前缀），
	// 与「服务暂不可用」是两类问题。给一个可分辨的码，前端与运维才能
	// 一眼看出「要改的是邮箱设置，不是程序」。
	CodeMailAuthFailed Code = 10009
)

// 认证与账号错误码。
const (
	CodeEmailExists        Code = 20001
	CodeInvalidCredentials Code = 20002
	CodeAccountDisabled    Code = 20003
	CodeAccountUnverified  Code = 20004
	CodeCodeInvalid        Code = 20005
	CodeCodeTooFrequent    Code = 20006
	CodeCodeDailyLimit     Code = 20007
	CodeTokenExpired       Code = 20008
	CodeTokenInvalid       Code = 20009
	CodeAccountLocked      Code = 20010
	CodePasswordTooWeak    Code = 20011
	CodePasswordSame       Code = 20012
	CodeOldPasswordWrong   Code = 20013
	CodeUserNotFound       Code = 20014
	CodeRefreshInvalid     Code = 20015
)

// 公司 / 岗位 / 审核错误码。
const (
	CodeCompanyNotFound   Code = 30001
	CodeCompanyExists     Code = 30002
	CodeJobNotFound       Code = 30003
	CodeJobAlreadyHandled Code = 30004
	CodeJobStateInvalid   Code = 30005
	CodeJobExists         Code = 30006
	CodeCareersURLInvalid Code = 30007
)

// 爬虫错误码。
const (
	CodeCrawlerDisabled Code = 40001
	CodeRobotsDenied    Code = 40002
	CodeFetchFailed     Code = 40003
	CodeParseFailed     Code = 40004
	CodeRunInProgress   Code = 40005
	CodeCareersNotFound Code = 40006
	CodeRunNotFound     Code = 40007
)

// 系统配置错误码。
const (
	CodeSettingNotFound Code = 50001
	CodeSettingInvalid  Code = 50002
)

// Error 是带错误码与 HTTP 状态码的业务错误。
type Error struct {
	Code       Code
	HTTPStatus int
	Message    string
	Details    any
	cause      error
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is / errors.As。
func (e *Error) Unwrap() error { return e.cause }

// WithDetails 附加结构化明细（会出现在响应体的 details 字段）。
func (e *Error) WithDetails(details any) *Error {
	clone := *e
	clone.Details = details
	return &clone
}

// WithCause 附加底层错误（仅记录日志，不外泄给客户端）。
func (e *Error) WithCause(err error) *Error {
	clone := *e
	clone.cause = err
	return &clone
}

// WithMessage 覆盖默认提示文案。
func (e *Error) WithMessage(msg string) *Error {
	clone := *e
	clone.Message = msg
	return &clone
}

// New 构造一个业务错误。
func New(code Code, message string) *Error {
	return &Error{Code: code, HTTPStatus: httpStatusOf(code), Message: message}
}

// httpStatusOf 按错误码分段推断 HTTP 状态码。
func httpStatusOf(code Code) int {
	switch code {
	case CodeSuccess:
		return http.StatusOK
	case CodeInvalidParams, CodeCodeInvalid, CodePasswordTooWeak, CodePasswordSame,
		CodeOldPasswordWrong, CodeCareersURLInvalid, CodeSettingInvalid, CodeJobStateInvalid:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeInvalidCredentials, CodeTokenExpired, CodeTokenInvalid,
		CodeRefreshInvalid:
		return http.StatusUnauthorized
	case CodeForbidden, CodeAccountDisabled, CodeAccountUnverified:
		return http.StatusForbidden
	case CodeNotFound, CodeUserNotFound, CodeCompanyNotFound, CodeJobNotFound,
		CodeRunNotFound, CodeSettingNotFound, CodeCareersNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeEmailExists, CodeCompanyExists, CodeJobExists,
		CodeJobAlreadyHandled, CodeRunInProgress, CodeAccountLocked:
		return http.StatusConflict
	case CodeTooManyReqs, CodeCodeTooFrequent, CodeCodeDailyLimit:
		return http.StatusTooManyRequests
	case CodeUnavailable, CodeCrawlerDisabled, CodeMailAuthFailed:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// As 从错误链中提取业务错误。
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Is 判断错误是否属于某个错误码。
func Is(err error, code Code) bool {
	e, ok := As(err)
	return ok && e.Code == code
}

// —— 预定义错误 ——

var (
	ErrUnknown       = New(CodeUnknown, "服务异常，请稍后重试")
	ErrInvalidParams = New(CodeInvalidParams, "请求参数有误")
	ErrUnauthorized  = New(CodeUnauthorized, "请先登录")
	ErrForbidden     = New(CodeForbidden, "没有权限执行该操作")
	ErrNotFound      = New(CodeNotFound, "资源不存在")
	ErrConflict      = New(CodeConflict, "资源冲突")
	ErrTooManyReqs   = New(CodeTooManyReqs, "请求过于频繁，请稍后重试")
	ErrInternal      = New(CodeInternal, "服务器内部错误")
	ErrUnavailable   = New(CodeUnavailable, "服务暂不可用")
	// ErrMailAuthFailed 只作为「认证失败」的载码错误，调用方必须用
	// WithMessage 填入带排查步骤的可读文案（见 internal/service/mail_hint.go）。
	ErrMailAuthFailed = New(CodeMailAuthFailed, "SMTP 认证失败，请检查邮箱服务商要求的授权码与服务开关")

	ErrEmailExists        = New(CodeEmailExists, "该邮箱已被注册")
	ErrInvalidCredentials = New(CodeInvalidCredentials, "邮箱或密码错误")
	ErrAccountDisabled    = New(CodeAccountDisabled, "账号已被禁用，请联系管理员")
	ErrAccountUnverified  = New(CodeAccountUnverified, "邮箱尚未验证")
	ErrCodeInvalid        = New(CodeCodeInvalid, "验证码错误或已过期")
	ErrCodeTooFrequent    = New(CodeCodeTooFrequent, "验证码发送过于频繁，请稍后再试")
	ErrCodeDailyLimit     = New(CodeCodeDailyLimit, "今日验证码发送次数已达上限")
	ErrTokenExpired       = New(CodeTokenExpired, "登录已过期，请重新登录")
	ErrTokenInvalid       = New(CodeTokenInvalid, "身份凭证无效")
	ErrAccountLocked      = New(CodeAccountLocked, "账号因多次登录失败已被临时锁定")
	ErrPasswordTooWeak    = New(CodePasswordTooWeak, "密码强度不足")
	ErrPasswordSame       = New(CodePasswordSame, "新密码不能与当前密码相同")
	ErrOldPasswordWrong   = New(CodeOldPasswordWrong, "原密码不正确")
	ErrUserNotFound       = New(CodeUserNotFound, "用户不存在")
	ErrRefreshInvalid     = New(CodeRefreshInvalid, "刷新令牌无效或已失效")

	ErrCompanyNotFound   = New(CodeCompanyNotFound, "公司不存在")
	ErrCompanyExists     = New(CodeCompanyExists, "公司已存在")
	ErrJobNotFound       = New(CodeJobNotFound, "岗位不存在")
	ErrJobAlreadyHandled = New(CodeJobAlreadyHandled, "该岗位已被审核，不能重复审核")
	ErrJobStateInvalid   = New(CodeJobStateInvalid, "当前状态不允许该操作")
	ErrJobExists         = New(CodeJobExists, "岗位已存在")
	ErrCareersURLInvalid = New(CodeCareersURLInvalid, "招聘页地址无效")

	ErrCrawlerDisabled = New(CodeCrawlerDisabled, "爬虫功能当前已关闭")
	ErrRobotsDenied    = New(CodeRobotsDenied, "目标站点 robots.txt 不允许抓取")
	ErrFetchFailed     = New(CodeFetchFailed, "抓取目标页面失败")
	ErrParseFailed     = New(CodeParseFailed, "解析页面内容失败")
	ErrRunInProgress   = New(CodeRunInProgress, "该公司的抓取任务正在执行中")
	ErrCareersNotFound = New(CodeCareersNotFound, "未能定位到该公司的招聘页面入口")
	ErrRunNotFound     = New(CodeRunNotFound, "抓取记录不存在")

	ErrSettingNotFound = New(CodeSettingNotFound, "配置项不存在")
	ErrSettingInvalid  = New(CodeSettingInvalid, "配置项内容不合法")
)

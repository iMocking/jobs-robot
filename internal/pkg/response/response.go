// Package response 定义统一的 HTTP 响应封装与辅助函数。
package response

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// RequestIDKey 是 gin.Context 中存放请求 ID 的键。
const RequestIDKey = "request_id"

// RequestIDHeader 是请求/响应中携带链路 ID 的头。
const RequestIDHeader = "X-Request-Id"

// Envelope 是所有接口统一的响应结构。
type Envelope struct {
	Code      apperr.Code `json:"code"`
	Message   string      `json:"message"`
	Data      any         `json:"data,omitempty"`
	Details   any         `json:"details,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
}

// Page 是分页响应的数据体。
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Pages    int `json:"pages"`
}

// NewPage 构造分页结果，自动计算总页数。
func NewPage[T any](items []T, total, page, pageSize int) Page[T] {
	if items == nil {
		items = []T{}
	}
	pages := 0
	if pageSize > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	return Page[T]{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}
}

func requestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return c.GetHeader(RequestIDHeader)
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:      apperr.CodeSuccess,
		Message:   "ok",
		Data:      data,
		RequestID: requestID(c),
	})
}

// Created 返回创建成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{
		Code:      apperr.CodeSuccess,
		Message:   "created",
		Data:      data,
		RequestID: requestID(c),
	})
}

// NoContent 返回无数据成功响应（仍保持统一信封结构，便于前端统一处理）。
func NoContent(c *gin.Context) {
	c.JSON(http.StatusOK, Envelope{
		Code:      apperr.CodeSuccess,
		Message:   "ok",
		RequestID: requestID(c),
	})
}

// Fail 返回业务错误；非业务错误统一降级为内部错误，避免泄漏实现细节。
func Fail(c *gin.Context, err error) {
	biz, ok := apperr.As(err)
	if !ok {
		biz = apperr.ErrInternal.WithCause(err)
	}
	env := Envelope{
		Code:      biz.Code,
		Message:   biz.Message,
		Details:   biz.Details,
		RequestID: requestID(c),
	}
	// 5xx 错误记录完整错误链，便于排查。
	if biz.HTTPStatus >= http.StatusInternalServerError {
		slog.Error("请求处理失败",
			"request_id", requestID(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"code", int(biz.Code),
			"error", errText(err),
		)
	}
	c.AbortWithStatusJSON(biz.HTTPStatus, env)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	var e *apperr.Error
	if errors.As(err, &e) && e.Unwrap() != nil {
		return e.Unwrap().Error()
	}
	return err.Error()
}

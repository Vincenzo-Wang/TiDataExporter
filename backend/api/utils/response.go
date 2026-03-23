package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 错误码定义
const (
	CodeSuccess         = 0
	CodeBadRequest      = 40001 // 参数验证失败
	CodeUnauthorized    = 40101 // API Key 或 Secret 错误
	CodeTenantDisabled  = 40102 // 租户已被禁用
	CodeForbidden       = 40301 // 权限不足
	CodeNotFound        = 40401 // 资源不存在
	CodeQuotaExceeded   = 40901 // 配额超限
	CodeRateLimited     = 42901 // 请求频率超限
	CodeInternalError   = 50001 // 服务器内部错误
)

// Response 标准响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	})
}

// SuccessWithMessage 成功响应（自定义消息）
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: message,
		Data:    data,
	})
}

// SuccessWithMessageAndCode 成功响应（自定义消息和数据）
func SuccessWithMessageAndCode(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: message,
		Data:    data,
	})
}

// Created 创建成功响应
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    CodeSuccess,
		Message: "created",
		Data:    data,
	})
}

// Accepted 已接受响应（异步任务）
func Accepted(c *gin.Context, data interface{}) {
	c.JSON(http.StatusAccepted, Response{
		Code:    CodeSuccess,
		Message: "accepted",
		Data:    data,
	})
}

// BadRequest 参数错误响应
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, Response{
		Code:    CodeBadRequest,
		Message: message,
	})
}

// BadRequestWithData 参数错误响应（带详细数据）
func BadRequestWithData(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusBadRequest, Response{
		Code:    CodeBadRequest,
		Message: message,
		Data:    data,
	})
}

// Unauthorized 未授权响应
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    CodeUnauthorized,
		Message: message,
	})
}

// TenantDisabled 租户已禁用响应
func TenantDisabled(c *gin.Context) {
	c.JSON(http.StatusForbidden, Response{
		Code:    CodeTenantDisabled,
		Message: "租户已被禁用",
	})
}

// Forbidden 权限不足响应
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:    CodeForbidden,
		Message: message,
	})
}

// NotFound 资源不存在响应
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Code:    CodeNotFound,
		Message: message,
	})
}

// QuotaExceeded 配额超限响应
func QuotaExceeded(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, Response{
		Code:    CodeQuotaExceeded,
		Message: message,
	})
}

// RateLimited 请求频率超限响应
func RateLimited(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, Response{
		Code:    CodeRateLimited,
		Message: message,
	})
}

// InternalError 服务器内部错误响应
func InternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, Response{
		Code:    CodeInternalError,
		Message: message,
	})
}

// Error 通用错误响应
func Error(c *gin.Context, code int, message string) {
	var statusCode int
	switch code {
	case CodeBadRequest:
		statusCode = http.StatusBadRequest
	case CodeUnauthorized, CodeTenantDisabled:
		statusCode = http.StatusUnauthorized
	case CodeForbidden:
		statusCode = http.StatusForbidden
	case CodeNotFound:
		statusCode = http.StatusNotFound
	case CodeQuotaExceeded:
		statusCode = http.StatusConflict
	case CodeRateLimited:
		statusCode = http.StatusTooManyRequests
	default:
		statusCode = http.StatusInternalServerError
	}
	c.JSON(statusCode, Response{
		Code:    code,
		Message: message,
	})
}

// PagedData 分页数据结构
type PagedData struct {
	Total int64       `json:"total"`
	Items interface{} `json:"items"`
}

// PagedSuccess 分页成功响应
func PagedSuccess(c *gin.Context, total int64, items interface{}) {
	Success(c, PagedData{
		Total: total,
		Items: items,
	})
}

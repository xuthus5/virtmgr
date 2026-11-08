package httpx

import (
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"

	"virtmgr/internal/domain"

	"github.com/go-chi/chi/v5/middleware"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

func JSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func DecodeJSON(r *stdhttp.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return errors.Join(domain.ErrInvalidInput, err)
	}
	return nil
}

func Error(w stdhttp.ResponseWriter, r *stdhttp.Request, err error) {
	status, code, message := mapError(err)
	JSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, RequestID: middleware.GetReqID(r.Context())}})
}

func mapError(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrUnauthorized):
		return stdhttp.StatusUnauthorized, "UNAUTHORIZED", "用户名或密码错误"
	case errors.Is(err, domain.ErrForbidden):
		return stdhttp.StatusForbidden, "FORBIDDEN", "没有执行该操作的权限"
	case errors.Is(err, domain.ErrReadOnly):
		return stdhttp.StatusForbidden, "READ_ONLY", "系统当前处于只读模式"
	case errors.Is(err, domain.ErrCSRF):
		return stdhttp.StatusForbidden, "CSRF_INVALID", "安全校验失败，请刷新页面后重试"
	case errors.Is(err, domain.ErrActionNotAllowed):
		return stdhttp.StatusConflict, "ACTION_NOT_ALLOWED", "当前虚拟机状态不允许执行该操作"
	case errors.Is(err, domain.ErrDiskResize):
		return stdhttp.StatusConflict, "DISK_RESIZE_UNSUPPORTED", "该磁盘不是可扩容的文件型 qcow2/raw 卷，或当前存储后端不支持扩容"
	case errors.Is(err, domain.ErrVolumeInUse):
		return stdhttp.StatusConflict, "VOLUME_IN_USE", "存储卷仍被虚拟机使用"
	case errors.Is(err, domain.ErrSnapshotRisky):
		return stdhttp.StatusConflict, "SNAPSHOT_REVERT_RISKY", "回滚该快照可能丢失当前状态，请先关闭虚拟机后重试"
	case errors.Is(err, domain.ErrConsoleUnavailable):
		return stdhttp.StatusConflict, "CONSOLE_UNAVAILABLE", "当前虚拟机没有可用的 VNC 控制台"
	case errors.Is(err, domain.ErrConsoleToken):
		return stdhttp.StatusUnauthorized, "CONSOLE_TOKEN_INVALID", "控制台凭证无效或已过期，请重新打开控制台"
	case errors.Is(err, domain.ErrNetworkActive):
		return stdhttp.StatusConflict, "NETWORK_ACTIVE", "网络正在运行，请先停止网络再修改配置"
	case errors.Is(err, domain.ErrNetworkUnsupported):
		return stdhttp.StatusConflict, "NETWORK_UPDATE_UNSUPPORTED", "当前网络类型不支持修改配置"
	case errors.Is(err, domain.ErrSelfModification):
		return stdhttp.StatusConflict, "SELF_MODIFICATION", "不能修改当前登录账号的角色或状态"
	case errors.Is(err, domain.ErrNotFound):
		return stdhttp.StatusNotFound, "NOT_FOUND", "请求的资源不存在"
	case errors.Is(err, domain.ErrConflict):
		return stdhttp.StatusConflict, "CONFLICT", "资源状态已发生变化"
	case errors.Is(err, domain.ErrInvalidInput):
		return stdhttp.StatusBadRequest, "INVALID_INPUT", "请求参数不合法"
	case errors.Is(err, domain.ErrTooLarge):
		return stdhttp.StatusRequestEntityTooLarge, "TOO_LARGE", "上传内容超过允许的最大大小"
	case errors.Is(err, domain.ErrUnavailable):
		return stdhttp.StatusServiceUnavailable, "UNAVAILABLE", "后端服务暂不可用"
	default:
		return stdhttp.StatusInternalServerError, "INTERNAL_ERROR", "服务内部错误"
	}
}

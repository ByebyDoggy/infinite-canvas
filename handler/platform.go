package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tigerowo/infinite-canvas/service"
)

type platformSessionRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// PlatformSessionConfig 返回前端拼授权页所需的最小信息。
// 这个端点必须在登录守卫之外：最需要它的正是还没有会话的游客。
func PlatformSessionConfig(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]any{
		"appKey":       service.PlatformAppKey(),
		"authorizeUrl": service.PlatformAuthorizeURL(),
		"platformBase": service.PlatformBaseURL(),
		"configured":   service.PlatformConfigured(),
	})
}

// PlatformSession 用一次性启动码换取本应用的会话。
// code 由前端从地址栏拿到后立刻从 history 抹掉，再 POST 到这里。
func PlatformSession(w http.ResponseWriter, r *http.Request) {
	var request platformSessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&request); err != nil {
		Fail(w, "登录请求无效")
		return
	}
	session, err := service.ExchangePlatformSession(strings.TrimSpace(request.Code))
	if err != nil {
		// 失败一律回首页而不是死胡同：带上原因让用户看到应用本身是好的。
		Fail(w, service.PlatformUserMessage(err))
		return
	}
	OK(w, session)
}

// PlatformSessionStatus 返回当前用户在本应用下的平台会话状态（不含密钥）。
func PlatformSessionStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录或权限不足")
		return
	}
	status, err := service.CurrentPlatformStatus(r.Context(), user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, status)
}

// PlatformSessionQuota 按用户操作触发一次额度刷新，不做轮询。
func PlatformSessionQuota(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录或权限不足")
		return
	}
	link, err := service.RefreshPlatformQuota(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, service.PlatformStatusOf(link))
}

// PlatformSessionDisconnect 只清本应用自己的会话。
// 契约硬约束 §4：绝不在应用内退出时删平台密钥，否则用户以为收回了，实际会被静默重签。
func PlatformSessionDisconnect(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录或权限不足")
		return
	}
	if err := service.UnlinkPlatformSession(user.ID); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

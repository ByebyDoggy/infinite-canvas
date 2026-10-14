package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

// 平台错误码。契约 §5 列出的 code 一律按原样透出；响应里没有 code 时归为
// platformUnspecified，绝不拿本地化的 message 做分支判断。
const (
	platformCodeNotFound          = "app.not_found"
	platformCodeDisabled          = "app.disabled"
	platformCodeNotAuthorized     = "app.not_authorized"
	platformCodeLaunchInvalid     = "app.launch_code_invalid"
	platformCodeLaunchExpired     = "app.launch_code_expired"
	platformCodeUserBanned        = "auth.user_banned"
	platformCodeUnspecified       = "app.error_unspecified"
	platformCodeUnreachable       = "app.platform_unreachable"
	platformCodeSessionRequired   = "app.session_required"
	platformCodeNoPlatformSession = "app.session_not_linked"
)

// PlatformError 平台调用失败。Code 只来自契约 §5，绝不从 message 推断。
type PlatformError struct {
	Code    string
	Message string
	Detail  string
}

func (err PlatformError) Error() string {
	if strings.TrimSpace(err.Detail) != "" {
		return err.Detail
	}
	return err.Message
}

func (err PlatformError) SafeMessage() string {
	return err.Message
}

// PlatformUserMessage 把平台错误翻译成给用户看的一句话，并明确下一步该找谁。
// 只有「换一张启动码能好」的情况才允许让用户回面板重开。
func PlatformUserMessage(err error) string {
	var platformErr PlatformError
	if !errors.As(err, &platformErr) {
		return "登录失败，请稍后重试"
	}
	switch platformErr.Code {
	case platformCodeLaunchInvalid, platformCodeLaunchExpired, platformCodeNotAuthorized:
		return "登录已失效，请在面板重新打开应用"
	case platformCodeNotFound, platformCodeDisabled:
		// 这两类是部署配置问题或管理员停用，换多少张启动码都没用。
		return "应用当前不可用，请联系平台管理员"
	case platformCodeUserBanned:
		return "账号已被封禁，请联系平台管理员"
	case platformCodeUnreachable:
		return "无法连接平台，请稍后重试"
	case platformCodeUnspecified:
		return "登录失败，请联系平台管理员"
	default:
		return "登录失败，请在面板重新打开应用"
	}
}

// PlatformClient 是新的 New API App Platform 客户端。它只做四件事：
// 用启动码换会话材料、拿用户额度、把上游 relay 翻译成 401 撤销信号、拼授权页地址。
type PlatformClient struct {
	httpClient *http.Client
}

var defaultPlatformClient = &PlatformClient{httpClient: &http.Client{Timeout: platformTimeout()}}

func platformTimeout() time.Duration {
	seconds := config.Cfg.PlatformTimeout
	if seconds <= 0 {
		seconds = 20
	}
	return time.Duration(seconds) * time.Second
}

func Platform() *PlatformClient {
	return defaultPlatformClient
}

// PlatformConfigured 判断是否已经配置平台基址与 app key。
func PlatformConfigured() bool {
	return strings.TrimSpace(config.Cfg.PlatformBaseURL) != "" && strings.TrimSpace(config.Cfg.PlatformAppKey) != ""
}

// PlatformAppKey 返回本应用在平台注册表里的 key，用于前端拼授权页。
func PlatformAppKey() string {
	return strings.TrimSpace(config.Cfg.PlatformAppKey)
}

// PlatformAuthorizeURL 返回平台托管授权页地址。优先用 exchange 回来并落库的地址，
// 其次用配置兜底，最后回落到基址拼接——前端永远不硬编码平台域名。
func PlatformAuthorizeURL() string {
	if configured := strings.TrimSpace(config.Cfg.PlatformAuthorizeURL); configured != "" {
		return configured
	}
	base := strings.TrimRight(strings.TrimSpace(config.Cfg.PlatformBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/apps/authorize"
}

type platformExchangeData struct {
	UserID         int    `json:"user_id"`
	Username       string `json:"username"`
	Role           int    `json:"role"`
	Language       string `json:"language"`
	AppKey         string `json:"app_key"`
	APIKey         string `json:"api_key"`
	APIBaseURL     string `json:"api_base_url"`
	AuthorizeURL   string `json:"authorize_url"`
	Quota          int64  `json:"quota"`
	UsedQuota      int64  `json:"used_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}

type platformEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
}

// Exchange 用一次性启动码换取会话材料。code 兑换后立即丢弃，不写日志、不落前端。
func (client *PlatformClient) Exchange(code string) (platformExchangeData, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return platformExchangeData{}, PlatformError{Code: platformCodeLaunchInvalid, Message: "登录已失效，请在面板重新打开应用"}
	}
	if !PlatformConfigured() {
		return platformExchangeData{}, PlatformError{Code: platformCodeNotFound, Message: "应用未配置平台地址", Detail: "APP_PLATFORM_BASE 或 APP_KEY 为空"}
	}
	payload, err := json.Marshal(map[string]string{
		"app_key": PlatformAppKey(),
		"code":    code,
	})
	if err != nil {
		return platformExchangeData{}, err
	}
	envelope, err := client.postJSON("/api/apps/exchange", payload)
	if err != nil {
		return platformExchangeData{}, err
	}
	if !envelope.Success {
		return platformExchangeData{}, classifyPlatformFailure(envelope)
	}
	var data platformExchangeData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return platformExchangeData{}, PlatformError{Code: platformCodeUnspecified, Message: "登录失败，请联系平台管理员", Detail: "平台返回的会话数据无法解析"}
	}
	if strings.TrimSpace(data.APIKey) == "" || strings.TrimSpace(data.APIBaseURL) == "" {
		return platformExchangeData{}, PlatformError{Code: platformCodeUnspecified, Message: "登录失败，请联系平台管理员", Detail: "平台返回的密钥或地址为空"}
	}
	return data, nil
}

// classifyPlatformFailure 把失败信封翻成 PlatformError。
// 契约要求用 code 分支；响应没有 code 时（平台当前多数失败路径只发 message）
// 归为 app.error_unspecified，把原文放进 Detail，绝不猜成某个具体业务错误。
func classifyPlatformFailure(envelope platformEnvelope) PlatformError {
	code := strings.TrimSpace(envelope.Code)
	if code == "" {
		return PlatformError{
			Code:    platformCodeUnspecified,
			Message: "登录失败，请联系平台管理员",
			Detail:  strings.TrimSpace(envelope.Message),
		}
	}
	return PlatformError{Code: code, Message: PlatformUserMessage(PlatformError{Code: code})}
}

func (client *PlatformClient) postJSON(path string, payload []byte) (platformEnvelope, error) {
	base := strings.TrimRight(strings.TrimSpace(config.Cfg.PlatformBaseURL), "/")
	if base == "" {
		return platformEnvelope{}, PlatformError{Code: platformCodeNotFound, Message: "应用未配置平台地址"}
	}
	request, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return platformEnvelope{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return platformEnvelope{}, PlatformError{Code: platformCodeUnreachable, Message: "无法连接平台，请稍后重试", Detail: err.Error()}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return platformEnvelope{}, PlatformError{
			Code:    platformCodeUnreachable,
			Message: "无法连接平台，请稍后重试",
			Detail:  fmt.Sprintf("平台返回 HTTP %d", response.StatusCode),
		}
	}
	var envelope platformEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return platformEnvelope{}, PlatformError{Code: platformCodeUnspecified, Message: "登录失败，请联系平台管理员", Detail: "平台返回的内容不是 JSON"}
	}
	return envelope, nil
}

// LinkPlatformSession 把 exchange 结果落到用户与绑定记录上：
// 已绑定过的平台用户复用原账号，新用户建本地账号，然后把密钥写进独立表。
func LinkPlatformSession(data platformExchangeData) (model.AuthUser, error) {
	if data.UserID <= 0 {
		return model.AuthUser{}, PlatformError{Code: platformCodeUnspecified, Message: "登录失败，请联系平台管理员", Detail: "平台未返回 user_id"}
	}
	link, linked, err := repository.GetPlatformLinkByPlatformUser(data.UserID)
	if err != nil {
		return model.AuthUser{}, err
	}

	var user model.User
	if linked {
		existing, ok, err := repository.GetUserByID(link.UserID)
		if err != nil {
			return model.AuthUser{}, err
		}
		if ok {
			user = existing
		}
	}
	if user.ID == "" {
		user = model.User{
			ID:          newID("user"),
			Username:    platformUsername(data),
			DisplayName: strings.TrimSpace(data.Username),
			Role:        model.UserRoleUser,
			AffCode:     newAffCode(),
			Status:      model.UserStatusActive,
			CreatedAt:   now(),
		}
	} else if user.Status == model.UserStatusBan {
		return model.AuthUser{}, PlatformError{Code: platformCodeUserBanned, Message: "账号已被封禁，请联系平台管理员"}
	}
	user.DisplayName = firstNonEmpty(strings.TrimSpace(data.Username), user.DisplayName, user.Username)
	user.LastLoginAt = now()
	user.UpdatedAt = now()
	user, err = repository.SaveUser(user)
	if err != nil {
		return model.AuthUser{}, err
	}

	saved := model.PlatformLink{
		UserID:        user.ID,
		PlatformUser:  data.UserID,
		Username:      strings.TrimSpace(data.Username),
		Language:      strings.TrimSpace(data.Language),
		APIKey:        strings.TrimSpace(data.APIKey),
		APIBaseURL:    strings.TrimRight(strings.TrimSpace(data.APIBaseURL), "/"),
		AuthorizeURL:  strings.TrimSpace(data.AuthorizeURL),
		UnlimitedQuot: data.UnlimitedQuota,
		Quota:         data.Quota,
		UsedQuota:     data.UsedQuota,
		CreatedAt:     now(),
		UpdatedAt:     now(),
	}
	if linked && link.CreatedAt != "" {
		saved.CreatedAt = link.CreatedAt
	}
	if _, err := repository.SavePlatformLink(saved); err != nil {
		return model.AuthUser{}, err
	}
	return model.PublicUser(user), nil
}

// platformUsername 生成一个稳定的本地用户名。平台用户名可能为空或已被占用，
// 这里统一用 user_id 作为后缀，保证同一平台用户每次登录落到同一个账号。
func platformUsername(data platformExchangeData) string {
	base := strings.TrimSpace(data.Username)
	if base == "" || strings.ContainsAny(base, " \t\r\n") {
		base = "platform"
	}
	return fmt.Sprintf("%s-%d", base, data.UserID)
}

// PlatformSessionForUser 读取某个用户在本应用下的平台会话；未绑定时返回 false。
func PlatformSessionForUser(userID string) (model.PlatformLink, bool) {
	link, ok, err := repository.GetPlatformLink(userID)
	if err != nil || !ok {
		return model.PlatformLink{}, false
	}
	if strings.TrimSpace(link.APIKey) == "" || strings.TrimSpace(link.APIBaseURL) == "" {
		return model.PlatformLink{}, false
	}
	return link, true
}

// RevokePlatformSession 在 relay 返回 401 时立即失效该用户在本应用的全部平台会话。
// 契约里 401 是唯一的「授权已撤销」信号，此时不重试、不回退到别的凭据。
func RevokePlatformSession(userID string) error {
	return repository.DeletePlatformLink(userID)
}

// RefreshPlatformQuota 按用户操作触发额度刷新，不做轮询。
func RefreshPlatformQuota(userID string) (model.PlatformLink, error) {
	link, ok := PlatformSessionForUser(userID)
	if !ok {
		return model.PlatformLink{}, PlatformError{Code: platformCodeNoPlatformSession, Message: "尚未连接平台账号"}
	}
	envelope, err := defaultPlatformClient.getJSON("/api/usage/token/", link.APIKey, link.APIBaseURL)
	if err != nil {
		return link, err
	}
	if !envelope.Success {
		return link, PlatformError{Code: platformCodeUnspecified, Message: "额度查询失败，请稍后重试"}
	}
	var usage struct {
		Quota     int64 `json:"quota"`
		UsedQuota int64 `json:"used_quota"`
	}
	if err := json.Unmarshal(envelope.Data, &usage); err != nil {
		return link, PlatformError{Code: platformCodeUnspecified, Message: "额度查询失败，请稍后重试"}
	}
	link.Quota = usage.Quota
	link.UsedQuota = usage.UsedQuota
	link.UpdatedAt = now()
	saved, err := repository.SavePlatformLink(link)
	if err != nil {
		return link, err
	}
	return saved, nil
}

func (client *PlatformClient) getJSON(path string, apiKey string, base string) (platformEnvelope, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return platformEnvelope{}, PlatformError{Code: platformCodeUnspecified, Message: "平台地址缺失"}
	}
	request, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return platformEnvelope{}, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return platformEnvelope{}, PlatformError{Code: platformCodeUnreachable, Message: "无法连接平台，请稍后重试", Detail: err.Error()}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return platformEnvelope{}, PlatformError{Code: platformCodeUnreachable, Message: "额度查询失败，请稍后重试", Detail: fmt.Sprintf("平台返回 HTTP %d", response.StatusCode)}
	}
	var envelope platformEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return platformEnvelope{}, PlatformError{Code: platformCodeUnspecified, Message: "额度查询失败，请稍后重试"}
	}
	return envelope, nil
}

// PlatformRelayChannel 把用户的平台会话翻译成一个普通渠道，交给既有代理链路。
// 这样上游 relay 的 401/402/429/403 语义、流式转发和错误脱敏都不用重写一遍。
func PlatformRelayChannel(link model.PlatformLink) model.ModelChannel {
	return model.ModelChannel{
		ID:       "platform:" + link.UserID,
		Protocol: ModelChannelProtocolOpenAI,
		Name:     "New API 平台",
		BaseURL:  platformRelayBaseURL(link.APIBaseURL),
		APIKey:   link.APIKey,
		Enabled:  true,
	}
}

// platformRelayBaseURL 归一化 relay 基址。契约里 api_base_url 不含 /v1，
// 但 buildOpenAIModelChannelURL 会在缺少版本后缀时自动补 /v1，所以这里保持原样即可。
func platformRelayBaseURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	return trimmed
}

// PlatformBaseURL 暴露平台基址，供前端展示与排障。
func PlatformBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(config.Cfg.PlatformBaseURL), "/")
}

// PlatformStatusView 是给前端看的会话状态，永远不含密钥。
type PlatformStatusView struct {
	PlatformUserID int    `json:"platformUserId"`
	Username       string `json:"username"`
	Language       string `json:"language"`
	APIBaseURL     string `json:"apiBaseUrl"`
	AuthorizeURL   string `json:"authorizeUrl"`
	Quota          int64  `json:"quota"`
	UsedQuota      int64  `json:"usedQuota"`
	UnlimitedQuota bool   `json:"unlimitedQuota"`
	Linked         bool   `json:"linked"`
}

// PlatformStatusOf 构造不含密钥的状态视图。
func PlatformStatusOf(link model.PlatformLink) PlatformStatusView {
	return PlatformStatusView{
		PlatformUserID: link.PlatformUser,
		Username:       link.Username,
		Language:       link.Language,
		APIBaseURL:     link.APIBaseURL,
		AuthorizeURL:   firstNonEmpty(link.AuthorizeURL, PlatformAuthorizeURL()),
		Quota:          link.Quota,
		UsedQuota:      link.UsedQuota,
		UnlimitedQuota: link.UnlimitedQuot,
		Linked:         strings.TrimSpace(link.APIKey) != "",
	}
}

// CurrentPlatformStatus 读取当前用户在本应用下的平台会话状态。
func CurrentPlatformStatus(_ context.Context, userID string) (PlatformStatusView, error) {
	link, ok := PlatformSessionForUser(userID)
	if !ok {
		return PlatformStatusView{Linked: false, AuthorizeURL: PlatformAuthorizeURL()}, nil
	}
	return PlatformStatusOf(link), nil
}

// UnlinkPlatformSession 只解除本应用自己的绑定，不动平台侧密钥。
func UnlinkPlatformSession(userID string) error {
	return repository.DeletePlatformLink(userID)
}

// PlatformRelayFailureMessage 把 relay 的失败状态码翻译成给用户看的一句话。
// 四种状态码对应四件不同的事，处理方式不同（见 SKILL 硬约束 §3）。
func PlatformRelayFailureMessage(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "授权已失效，请在面板重新打开应用"
	case http.StatusPaymentRequired:
		return "平台额度不足，请在面板充值后重试"
	case http.StatusTooManyRequests:
		return "请求过于频繁，请稍后重试"
	case http.StatusForbidden:
		return "该模型不在本应用的白名单内，请联系平台管理员"
	default:
		return "模型调用失败，请稍后重试"
	}
}

// HandlePlatformRelayFailure 只看 401：那是契约里唯一的「授权已撤销」信号。
// 命中时立即失效该用户在本应用的全部会话，并且不重试、不回退到别的凭据。
// 402 / 429 / 403 都不动会话——额度不足或限流不代表授权没了。
func HandlePlatformRelayFailure(userID string, status int) bool {
	if status != http.StatusUnauthorized || strings.TrimSpace(userID) == "" {
		return false
	}
	if err := repository.DeletePlatformLink(userID); err != nil {
		return false
	}
	return true
}

// ExchangePlatformSession 是前端回调的唯一入口：用一次性启动码换到本应用会话。
// 顺序固定为 exchange → 落库 → 签发本应用 JWT；失败时把平台错误原样抛出，
// 由 handler 翻成用户可读的一句话。
func ExchangePlatformSession(code string) (model.AuthSession, error) {
	data, err := Platform().Exchange(code)
	if err != nil {
		return model.AuthSession{}, err
	}
	user, err := LinkPlatformSession(data)
	if err != nil {
		return model.AuthSession{}, err
	}
	token, err := newTokenForUserID(user.ID)
	if err != nil {
		return model.AuthSession{}, err
	}
	return model.AuthSession{Token: token, User: user}, nil
}

// PlatformModelChannelID 是平台模型列表在公开设置里的渠道标识。
// 前端把它当成一个系统渠道的 id，只用于模型选择与能力分类；
// 请求路由仍由服务端按平台会话解析，这个 id 不参与寻址。
const PlatformModelChannelID = "platform"

// platformModelCacheTTL 控制模型列表的缓存时长。页面每次加载都会取一次
// 公开设置，所以这里必须缓存，否则等于每次开页面都打一次平台。
const platformModelCacheTTL = 5 * time.Minute

type platformModelCacheEntry struct {
	models    []string
	expiresAt time.Time
}

var (
	platformModelCacheMu sync.Mutex
	platformModelCache   = map[string]platformModelCacheEntry{}
)

// PlatformModels 读取平台为该用户开放的模型列表。
// 取不到时返回空列表而不是错误：模型列表缺失不该让整个页面打不开。
func PlatformModels(link model.PlatformLink) []string {
	cacheKey := link.APIBaseURL + "|" + link.APIKey
	platformModelCacheMu.Lock()
	if entry, ok := platformModelCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		platformModelCacheMu.Unlock()
		return entry.models
	}
	platformModelCacheMu.Unlock()

	models := fetchPlatformModels(link)
	platformModelCacheMu.Lock()
	platformModelCache[cacheKey] = platformModelCacheEntry{models: models, expiresAt: time.Now().Add(platformModelCacheTTL)}
	platformModelCacheMu.Unlock()
	return models
}

func fetchPlatformModels(link model.PlatformLink) []string {
	base := strings.TrimRight(strings.TrimSpace(link.APIBaseURL), "/")
	if base == "" || strings.TrimSpace(link.APIKey) == "" {
		return nil
	}
	request, err := http.NewRequest(http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return nil
	}
	request.Header.Set("Authorization", "Bearer "+link.APIKey)
	response, err := defaultPlatformClient.httpClient.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		name := strings.TrimSpace(item.ID)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		models = append(models, name)
	}
	return models
}

// InvalidatePlatformModels 在会话失效或额度刷新后丢弃缓存的模型列表。
func InvalidatePlatformModels(link model.PlatformLink) {
	platformModelCacheMu.Lock()
	delete(platformModelCache, link.APIBaseURL+"|"+link.APIKey)
	platformModelCacheMu.Unlock()
}

// PlatformModelChannelSetting 用平台开放的模型列表替换公开设置里的渠道配置。
//
// 平台用户的可选模型只可能来自平台，所以这里直接接管整个 ModelChannel：
// 渠道列表收敛成一条平台渠道，AllowUserRemoteChannel 打开让前端走账号代理，
// AllowCustomChannel 关闭——否则默认的本地直连模式会看到一个空模型列表。
func PlatformModelChannelSetting(base model.PublicModelChannelSetting, link model.PlatformLink) model.PublicModelChannelSetting {
	models := PlatformModels(link)
	allowCustomChannel := false
	allowUserRemoteChannel := true
	base.AvailableModels = models
	base.AvailableWorkflows = []string{}
	base.Channels = []model.PublicModelChannelInfo{{
		ID:       PlatformModelChannelID,
		Protocol: ModelChannelProtocolOpenAI,
		Name:     "New API 平台",
		Models:   models,
		Weight:   1,
		Enabled:  true,
	}}
	base.DefaultModel = ""
	base.DefaultImageModel = ""
	base.DefaultVideoModel = ""
	base.DefaultTextModel = ""
	base.AllowCustomChannel = &allowCustomChannel
	base.AllowUserRemoteChannel = &allowUserRemoteChannel
	return base
}


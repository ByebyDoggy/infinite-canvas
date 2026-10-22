package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
)

func platformLinkFixture() model.PlatformLink {
	return model.PlatformLink{
		UserID:       "user-1",
		PlatformUser: 42,
		Username:     "creator",
		APIKey:       "sk-server-side",
		APIBaseURL:   "https://api.example.com",
	}
}

// withPlatformBase 指向一个测试服务器，并在用例结束后还原配置。
func withPlatformBase(t *testing.T, base string) {
	t.Helper()
	savedBase, savedKey := config.Cfg.PlatformBaseURL, config.Cfg.PlatformAppKey
	config.Cfg.PlatformBaseURL = base
	config.Cfg.PlatformAppKey = "canvas"
	t.Cleanup(func() {
		config.Cfg.PlatformBaseURL = savedBase
		config.Cfg.PlatformAppKey = savedKey
	})
}

func platformEnvelopeResponse(success bool, code string, message string, data any) string {
	body := map[string]any{"success": success, "message": message, "data": data}
	if code != "" {
		body["code"] = code
	}
	encoded, _ := json.Marshal(body)
	return string(encoded)
}

func TestExchangeSendsAppKeyAndCode(t *testing.T) {
	var gotAppKey, gotCode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/apps/exchange" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotAppKey, gotCode = payload["app_key"], payload["code"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(platformEnvelopeResponse(true, "", "", map[string]any{
			"user_id": 42, "username": "creator", "api_key": "sk-secret",
			"api_base_url": "https://api.example.com", "quota": 500000, "used_quota": 1000,
		})))
	}))
	defer server.Close()
	withPlatformBase(t, server.URL)

	data, err := Platform().Exchange("Kq7-once")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if gotAppKey != "canvas" || gotCode != "Kq7-once" {
		t.Fatalf("exchange payload = app_key %q code %q", gotAppKey, gotCode)
	}
	if data.APIKey != "sk-secret" || data.UserID != 42 {
		t.Fatalf("exchange data = %#v", data)
	}
}

// 契约要求用 code 分支；响应没有 code 时归为「平台没说原因」，
// 绝不能退化成拿 message 猜业务错误——那正是把「应用标识配错了」
// 显示成「启动码无效，请回面板重开」的成因。
func TestExchangeWithoutCodeIsNotClassifiedAsLaunchInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(platformEnvelopeResponse(false, "", "App not found", nil)))
	}))
	defer server.Close()
	withPlatformBase(t, server.URL)

	_, err := Platform().Exchange("Kq7-once")
	platformErr, ok := err.(PlatformError)
	if !ok {
		t.Fatalf("expected PlatformError, got %T", err)
	}
	if platformErr.Code != platformCodeUnspecified {
		t.Fatalf("code = %q, want %q", platformErr.Code, platformCodeUnspecified)
	}
	if platformErr.Detail != "App not found" {
		t.Fatalf("detail = %q, want the raw platform message", platformErr.Detail)
	}
	if strings.Contains(PlatformUserMessage(err), "重新打开应用") {
		t.Fatalf("unspecified failure must not tell the user to reopen from the panel: %q", PlatformUserMessage(err))
	}
}

func TestExchangeHonorsPlatformCode(t *testing.T) {
	for _, code := range []string{platformCodeNotFound, platformCodeDisabled, platformCodeLaunchExpired} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(platformEnvelopeResponse(false, code, "本地化文案", nil)))
			}))
			defer server.Close()
			withPlatformBase(t, server.URL)

			_, err := Platform().Exchange("Kq7-once")
			platformErr, ok := err.(PlatformError)
			if !ok {
				t.Fatalf("expected PlatformError, got %T", err)
			}
			if platformErr.Code != code {
				t.Fatalf("code = %q, want %q", platformErr.Code, code)
			}
		})
	}
}

// app.not_found 与 app.disabled 是部署配置问题与管理员停用，
// 换多少张启动码都没用，提示语不得让用户回面板重开。
func TestConfigAndDisabledFailuresDoNotAskForANewLaunchCode(t *testing.T) {
	for _, code := range []string{platformCodeNotFound, platformCodeDisabled} {
		message := PlatformUserMessage(PlatformError{Code: code})
		if strings.Contains(message, "重新打开应用") {
			t.Fatalf("%s message must not ask for a new launch code: %q", code, message)
		}
		if !strings.Contains(message, "管理员") {
			t.Fatalf("%s message must point at the administrator: %q", code, message)
		}
	}
	for _, code := range []string{platformCodeLaunchInvalid, platformCodeLaunchExpired, platformCodeNotAuthorized} {
		message := PlatformUserMessage(PlatformError{Code: code})
		if !strings.Contains(message, "重新打开应用") {
			t.Fatalf("%s message should let the user reopen from the panel: %q", code, message)
		}
	}
}

func TestExchangeRejectsEmptyCodeWithoutCallingPlatform(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()
	withPlatformBase(t, server.URL)

	if _, err := Platform().Exchange("   "); err == nil {
		t.Fatal("expected an error for an empty code")
	}
	if called {
		t.Fatal("empty code must not reach the platform")
	}
}

func TestPlatformAuthorizeURLFallsBackToBase(t *testing.T) {
	saved := config.Cfg.PlatformAuthorizeURL
	t.Cleanup(func() { config.Cfg.PlatformAuthorizeURL = saved })

	config.Cfg.PlatformAuthorizeURL = ""
	withPlatformBase(t, "https://ai.lesec.top/")
	if got := PlatformAuthorizeURL(); got != "https://ai.lesec.top/apps/authorize" {
		t.Fatalf("authorize url = %q", got)
	}

	config.Cfg.PlatformAuthorizeURL = "https://custom.example/apps/authorize"
	if got := PlatformAuthorizeURL(); got != "https://custom.example/apps/authorize" {
		t.Fatalf("configured authorize url should win, got %q", got)
	}
}

// 只有 401 会动授权项；其余状态码一律不碰凭据，也不碰数据库。
// 单张 grant 的 401 只降级这一条，是否登出由 platformGrantsExhausted 判定
// （纯函数，见下一个用例），这里只锁住「非 401 绝不动会话」。
func TestPlatformRelayFailureOnlyRevokesOn401(t *testing.T) {
	for _, status := range []int{
		http.StatusPaymentRequired,
		http.StatusTooManyRequests,
		http.StatusForbidden,
		http.StatusInternalServerError,
	} {
		if got := HandlePlatformRelayFailure("user-1", "auto:image", status); got {
			t.Fatalf("HandlePlatformRelayFailure(%d) must not revoke anything", status)
		}
	}
	if platformStatusRevokesGrant(http.StatusPaymentRequired) {
		t.Fatal("402 must not be treated as a revoked key")
	}
	if !platformStatusRevokesGrant(http.StatusUnauthorized) {
		t.Fatal("401 must be treated as a revoked key")
	}
}

// 单张密钥失效只降级它自己；只有全部授权项都失效才等同于用户断开连接。
func TestPlatformGrantsExhaustedRequiresEveryGrantToBeRevoked(t *testing.T) {
	grants := []model.PlatformGrant{
		{GrantKey: "auto:image", Status: model.PlatformGrantStatusActive},
		{GrantKey: "auto:text", Status: model.PlatformGrantStatusActive},
	}
	if platformGrantsExhausted(grants, "auto:image") {
		t.Fatal("revoking one grant must not sign the user out")
	}
	grants[1].Status = model.PlatformGrantStatusRevoked
	if !platformGrantsExhausted(grants, "auto:image") {
		t.Fatal("revoking the last active grant must sign the user out")
	}
	if !platformGrantsExhausted(nil, "auto:image") {
		t.Fatal("a user without grants has nothing left to revoke")
	}
}

func TestPlatformRelayFailureMessagesAreDistinct(t *testing.T) {
	messages := map[int]string{
		http.StatusUnauthorized:    "授权已失效，请在面板重新打开应用",
		http.StatusPaymentRequired: "平台额度不足，请在面板充值后重试",
		http.StatusTooManyRequests: "请求过于频繁，请稍后重试",
		http.StatusForbidden:       "该模型不在本应用的白名单内，请联系平台管理员",
	}
	seen := map[string]int{}
	for status, want := range messages {
		got := PlatformRelayFailureMessage(status)
		if got != want {
			t.Fatalf("PlatformRelayFailureMessage(%d) = %q, want %q", status, got, want)
		}
		if seen[got]++; seen[got] > 1 {
			t.Fatalf("status %d reuses another status's message: %q", status, got)
		}
	}
}

// 402 之后会话必须仍然有效：额度不足不代表授权没了。
func TestPaymentRequiredKeepsSessionUsable(t *testing.T) {
	if HandlePlatformRelayFailure("user-1", "auto:image", http.StatusPaymentRequired) {
		t.Fatal("402 must not invalidate the session")
	}
	if HandlePlatformRelayFailure("user-1", "auto:image", http.StatusTooManyRequests) {
		t.Fatal("429 must not invalidate the session")
	}
}

// PlatformStatusOf 永远不能把密钥带进前端视图。
func TestPlatformStatusViewNeverCarriesAPIKey(t *testing.T) {
	view := PlatformStatusOf(platformLinkFixture())
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal status view: %v", err)
	}
	if strings.Contains(string(encoded), "sk-") {
		t.Fatalf("status view leaked a key: %s", encoded)
	}
	if !view.Linked {
		t.Fatal("a linked session must report linked=true")
	}
}

func TestPlatformRelayChannelUsesServerSideKey(t *testing.T) {
	channel := PlatformRelayChannel(platformLinkFixture())
	if channel.APIKey != "sk-server-side" {
		t.Fatalf("channel key = %q", channel.APIKey)
	}
	if channel.BaseURL != "https://api.example.com" {
		t.Fatalf("channel base url = %q", channel.BaseURL)
	}
	if channel.Protocol != ModelChannelProtocolOpenAI {
		t.Fatalf("channel protocol = %q", channel.Protocol)
	}
}

func TestPlatformModelsReadsRelayModelList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-server-side" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"object":"list","data":[{"id":"gpt-image-2"},{"id":"gpt-image-2.5-flare"},{"id":"gpt-image-2"},{"id":""}]}`))
	}))
	defer server.Close()

	link := platformLinkFixture()
	link.APIBaseURL = server.URL
	models := PlatformModels(link)
	if len(models) != 2 || models[0] != "gpt-image-2" || models[1] != "gpt-image-2.5-flare" {
		t.Fatalf("models = %#v, want the deduplicated platform list", models)
	}
}

// 模型列表取不到时必须是空列表而不是错误：缺列表不该让整个页面打不开。
func TestPlatformModelsDegradesToEmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	link := platformLinkFixture()
	link.APIBaseURL = server.URL
	if models := PlatformModels(link); len(models) != 0 {
		t.Fatalf("upstream error must degrade to empty, got %#v", models)
	}

	link.APIBaseURL = "http://127.0.0.1:1"
	if models := PlatformModels(link); len(models) != 0 {
		t.Fatalf("unreachable platform must degrade to empty, got %#v", models)
	}
}

func TestPlatformModelChannelSettingTakesOverPublicChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2.5-sunburst"}]}`))
	}))
	defer server.Close()

	link := platformLinkFixture()
	link.APIBaseURL = server.URL

	// 基线故意留一份「管理员配了别的渠道」的配置，确认平台身份会整体接管。
	allowCustom := true
	adminConfigured := model.PublicModelChannelSetting{
		AvailableModels:    []string{"some-other-model"},
		DefaultImageModel:  "some-other-model",
		AllowCustomChannel: &allowCustom,
	}

	got := PlatformModelChannelSetting(adminConfigured, link)

	assertStringSlice(t, got.AvailableModels, []string{"gpt-image-2.5-sunburst"})
	assertStringSlice(t, got.AvailableWorkflows, []string{})
	if len(got.Channels) != 1 {
		t.Fatalf("channels = %#v, want exactly the platform channel", got.Channels)
	}
	channel := got.Channels[0]
	if channel.ID != PlatformModelChannelID || channel.Protocol != "openai" {
		t.Fatalf("platform channel = %#v", channel)
	}
	assertStringSlice(t, channel.Models, []string{"gpt-image-2.5-sunburst"})
	// 默认模型留空，交给前端按能力挑：平台不保证哪个模型一定是生图模型。
	for name, value := range map[string]string{
		"DefaultModel":      got.DefaultModel,
		"DefaultImageModel": got.DefaultImageModel,
		"DefaultVideoModel": got.DefaultVideoModel,
		"DefaultTextModel":  got.DefaultTextModel,
	} {
		if value != "" {
			t.Fatalf("%s = %q, want empty so the frontend picks by capability", name, value)
		}
	}
	// 关掉本地直连并打开云端渠道：否则前端默认落在 local 模式，看到的还是空列表。
	if got.AllowCustomChannel == nil || *got.AllowCustomChannel {
		t.Fatal("AllowCustomChannel must be false for platform users")
	}
	if got.AllowUserRemoteChannel == nil || !*got.AllowUserRemoteChannel {
		t.Fatal("AllowUserRemoteChannel must be true for platform users")
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

// SelectPlatformGrant 是应用侧跨密钥路由的核心：模型命中哪条授权项，
// 就用哪张密钥发请求。多条命中时按 priority 升序，结果必须确定。
func TestSelectPlatformGrantPicksByModelAndPriority(t *testing.T) {
	grants := []model.PlatformGrant{
		{GrantKey: "auto:text", Priority: 1, Models: []string{"glm-5.3"}, Status: model.PlatformGrantStatusActive},
		{GrantKey: "auto:image", Priority: 0, Models: []string{"gpt-image-2.5", "gpt-image-2"}, Status: model.PlatformGrantStatusActive},
	}
	grant, ok := SelectPlatformGrant(grants, "gpt-image-2.5")
	if !ok || grant.GrantKey != "auto:image" {
		t.Fatalf("gpt-image-2.5 should route to auto:image, got %#v ok=%v", grant, ok)
	}
	grant, ok = SelectPlatformGrant(grants, "glm-5.3")
	if !ok || grant.GrantKey != "auto:text" {
		t.Fatalf("glm-5.3 should route to auto:text, got %#v ok=%v", grant, ok)
	}
	if _, ok := SelectPlatformGrant(grants, "not-granted"); ok {
		t.Fatal("a model outside every grant must not route to any key")
	}

	// 被降级的授权项不再参与路由，但其他授权项照常可用。
	grants[1].Status = model.PlatformGrantStatusRevoked
	if _, ok := SelectPlatformGrant(grants, "gpt-image-2.5"); ok {
		t.Fatal("a revoked grant must not be selected")
	}

	// 声明面为空的授权项（v1 legacy）兜底，且永远排在声明过模型的授权项之后。
	legacy := []model.PlatformGrant{
		{GrantKey: model.PlatformGrantKeyLegacy, Priority: 0, Status: model.PlatformGrantStatusActive},
		{GrantKey: "auto:image", Priority: 5, Models: []string{"gpt-image-2.5"}, Status: model.PlatformGrantStatusActive},
	}
	grant, ok = SelectPlatformGrant(legacy, "gpt-image-2.5")
	if !ok || grant.GrantKey != "auto:image" {
		t.Fatalf("an explicit grant must win over the legacy fallback, got %#v", grant)
	}
	grant, ok = SelectPlatformGrant(legacy, "some-other-model")
	if !ok || grant.GrantKey != model.PlatformGrantKeyLegacy {
		t.Fatalf("an undeclared model must fall back to the legacy grant, got %#v ok=%v", grant, ok)
	}
}

// 路由出来的渠道必须带该授权项自己的密钥与 grant 标记：
// 401 降级、用量归属、日志排障都依赖这个标记。
func TestPlatformRelayChannelForModelCarriesGrantKey(t *testing.T) {
	link := platformLinkFixture()
	grants := []model.PlatformGrant{
		{GrantKey: "auto:image", Label: "图像生成", Group: "GPT-Image-2.5", Priority: 0, Models: []string{"gpt-image-2.5"}, APIKey: "sk-image", Status: model.PlatformGrantStatusActive},
	}
	channel, ok := PlatformRelayChannelForModel(link, grants, "gpt-image-2.5")
	if !ok {
		t.Fatal("gpt-image-2.5 must resolve to the image grant")
	}
	if channel.APIKey != "sk-image" {
		t.Fatalf("channel must use the grant key, got %q", channel.APIKey)
	}
	if channel.PlatformGrantKey != "auto:image" {
		t.Fatalf("channel must carry the grant key, got %q", channel.PlatformGrantKey)
	}
	if channel.Name != "New API 平台 · 图像生成" {
		t.Fatalf("channel name should identify the grant, got %q", channel.Name)
	}
	if channel.BaseURL != link.APIBaseURL {
		t.Fatalf("all grants share the relay base url, got %q", channel.BaseURL)
	}
	if _, ok := PlatformRelayChannelForModel(link, grants, "unknown-model"); ok {
		t.Fatal("an ungranted model must not resolve to a channel")
	}
}

// exchange 的 grants 是权威数据；平台只给顶层 api_key（v1）时要落成一条
// legacy 授权项，保证老部署仍然能用。
func TestPlatformGrantsFromExchangeFallsBackToLegacyKey(t *testing.T) {
	v3 := platformGrantsFromExchange("user-1", platformExchangeData{
		APIKey: "sk-primary",
		Grants: []platformExchangeGrant{
			{ID: "auto:image", Label: "图像生成", Group: "GPT-Image-2.5", Models: []string{"gpt-image-2.5", " gpt-image-2.5 "}, Priority: 0, APIKey: "sk-image"},
			{ID: "", APIKey: "sk-anonymous", Priority: 1},
			{ID: "auto:empty", APIKey: ""},
		},
	})
	if len(v3) != 2 {
		t.Fatalf("grants without a key must be dropped, got %#v", v3)
	}
	assertStringSlice(t, v3[0].Models, []string{"gpt-image-2.5"})
	if v3[1].GrantKey == "" {
		t.Fatal("a grant without an id must still get a stable local key")
	}

	legacy := platformGrantsFromExchange("user-1", platformExchangeData{APIKey: "sk-primary"})
	if len(legacy) != 1 || legacy[0].GrantKey != model.PlatformGrantKeyLegacy {
		t.Fatalf("a v1 exchange must produce exactly one legacy grant, got %#v", legacy)
	}
	if legacy[0].Status != model.PlatformGrantStatusActive {
		t.Fatalf("a freshly issued grant must be active, got %q", legacy[0].Status)
	}
}

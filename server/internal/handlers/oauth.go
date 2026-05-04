package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/auth"
	"github.com/youruser/taskflow/internal/oauth"
	"github.com/youruser/taskflow/internal/store"
)

// OAuthHandler 接外部 OAuth2 / OIDC 认证中心。
//
// 端点:
//   - GET  /api/auth/oauth/start     —— 浏览器入口,生成 PKCE,302 到认证中心。
//     支持 ?client=web|desktop|android & ?device_id=xxx。
//   - GET  /api/auth/oauth/callback  —— 认证中心回调。完成 code 交换 + userinfo。
//     client=web    -> 302 到 ${frontend}/oauth/callback#code=...
//     client=desktop|android -> 302 到 ${PUBLIC_BASE_URL}/oauth/done
//     (静态成功页,提示用户回到客户端);handoff 通过 device_id 索引,
//     客户端走 /api/auth/oauth/poll 取走。
//   - GET  /api/auth/oauth/poll      —— 桌面 / Android 客户端轮询接口。
//     ?device_id=xxx -> {handoff_code} 或 204。
//   - POST /api/auth/oauth/finalize  —— 用 handoff code 换本服务的 access/refresh JWT。
//   - GET  /api/auth/oauth/done      —— 静态登录成功页(桌面 / Android 用户登录后看到的"请回客户端"提示)。
//
// 还有一个公开的 GET /api/auth/config —— 前端登录页用它判断是否启用了 OAuth。
type OAuthHandler struct {
	Provider      *oauth.Provider
	Pending       *oauth.PendingStore
	Issuer        *auth.Issuer
	Users         *store.UserStore
	RefreshTokens *store.RefreshTokenStore
	Logger        *slog.Logger
}

func NewOAuthHandler(
	provider *oauth.Provider,
	pending *oauth.PendingStore,
	issuer *auth.Issuer,
	users *store.UserStore,
	refresh *store.RefreshTokenStore,
	logger *slog.Logger,
) *OAuthHandler {
	return &OAuthHandler{
		Provider:      provider,
		Pending:       pending,
		Issuer:        issuer,
		Users:         users,
		RefreshTokens: refresh,
		Logger:        logger,
	}
}

// normalizeClientKind 把任意 client 入参规范成 "web" / "desktop" / "android"。
// 未知值视为 "web",兼容旧客户端。
func normalizeClientKind(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "desktop", "windows", "tauri":
		return "desktop"
	case "android", "mobile":
		return "android"
	default:
		return "web"
	}
}

// Start GET /api/auth/oauth/start?device_id=xxx&client=web|desktop|android
//
// 生成 state + PKCE,302 到认证中心。
//
//   - device_id 可选;桌面 / 移动端必填(用作 poll 的 key,推荐 32 字节随机)。
//   - client    决定回调阶段的重定向方式;web 走重定向 + 前端 finalize,
//     desktop/android 走"系统浏览器 + 服务端 poll"。
func (h *OAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	deviceID := strings.TrimSpace(q.Get("device_id"))
	clientKind := normalizeClientKind(q.Get("client"))

	// 桌面 / 移动端没有 device_id 就 poll 不到 —— 这是配置错误,直接报错。
	if (clientKind == "desktop" || clientKind == "android") && deviceID == "" {
		writeError(w, http.StatusBadRequest, "missing_device_id",
			"client="+clientKind+" requires device_id (>=32 random chars)")
		return
	}

	state, _, challenge, err := h.Pending.SaveState(deviceID, clientKind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	authURL := h.Provider.AuthorizeURLFor(state, challenge)
	// 302 兼容性最好,303 更精确但本接口都是 GET,用 302 即可。
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback GET /api/auth/oauth/callback?code=&state=
//
// 失败一律按 client 类型走对应的失败展示:
//   - web    -> 302 到 ${frontend}/oauth/callback#error=...
//   - desktop/android -> 302 到 ${PUBLIC_BASE_URL}/oauth/done?error=...
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// 认证中心可能直接报错(用户拒绝、配置错误等)。
	if e := q.Get("error"); e != "" {
		desc := q.Get("error_description")
		// 这种情况下 state 也可能为空,只能按通用方式跳到 web 回调页 —— 用户在浏览器里。
		h.redirectAfterCallback(w, r, oauth.PendingState{ClientKind: "web"}, "", e, desc)
		return
	}

	state := q.Get("state")
	code := q.Get("code")
	if state == "" || code == "" {
		h.redirectAfterCallback(w, r, oauth.PendingState{ClientKind: "web"}, "", "invalid_request", "missing code or state")
		return
	}
	pending, ok := h.Pending.LookupState(state)
	if !ok {
		h.redirectAfterCallback(w, r, oauth.PendingState{ClientKind: "web"}, "", "invalid_state", "state expired or unknown — 请回到登录页重新发起")
		return
	}

	// 1) code -> token
	tokCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := h.Provider.Exchange(tokCtx, code, pending.CodeVerifier)
	if err != nil {
		h.logger().Warn("oauth token exchange failed", "err", err)
		h.redirectAfterCallback(w, r, pending, "", "token_exchange_failed", err.Error())
		return
	}

	// 2) access_token -> userinfo, 并用 OIDC id_token 兜底真实邮箱/姓名
	uiCtx, cancel2 := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel2()
	info, err := h.Provider.ResolveUserInfo(uiCtx, tok)
	if err != nil {
		h.logger().Warn("oauth userinfo failed", "err", err)
		h.redirectAfterCallback(w, r, pending, "", "userinfo_failed", err.Error())
		return
	}

	// 3) upsert 本地用户
	user, err := h.Users.UpsertOAuth(r.Context(), h.Provider.Name, info.Subject, info.Email, info.DisplayName)
	if err != nil {
		h.logger().Error("oauth upsert user", "err", err, "sub", info.Subject)
		h.redirectAfterCallback(w, r, pending, "", "user_upsert_failed", err.Error())
		return
	}

	// 4) 给客户端一个一次性 handoff code。
	handoff, err := h.Pending.SaveHandoff(user.ID, pending.DeviceID)
	if err != nil {
		h.redirectAfterCallback(w, r, pending, "", "internal", err.Error())
		return
	}

	// 桌面 / Android:把 handoff 通过 device_id 索引,等客户端 poll 取走
	if pending.ClientKind == "desktop" || pending.ClientKind == "android" {
		h.Pending.LinkDeviceHandoff(pending.DeviceID, handoff)
	}

	h.redirectAfterCallback(w, r, pending, handoff, "", "")
}

// Finalize POST /api/auth/oauth/finalize  body: {"code": "..."}
//
// 用 handoff code 换本服务的 access + refresh token。三端登录的最终一站。
type oauthFinalizeRequest struct {
	Code string `json:"code"`
}

func (h *OAuthHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	var req oauthFinalizeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "code required")
		return
	}
	sess, ok := h.Pending.LookupHandoff(req.Code)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_handoff", "handoff code expired or already used")
		return
	}
	user, err := h.Users.GetByID(r.Context(), sess.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid_handoff", "user no longer exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if user.IsDisabled {
		writeError(w, http.StatusUnauthorized, "account_disabled", "account disabled, contact admin")
		return
	}

	now := time.Now().UTC()
	access, accessExp, err := h.Issuer.IssueAccessToken(user.ID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	refresh, refreshHash, refreshExp, err := h.Issuer.IssueRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := h.RefreshTokens.Create(r.Context(), user.ID, refreshHash, sess.DeviceID, refreshExp); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:           access,
		AccessTokenExpiresAt:  accessExp.UTC().Format(time.RFC3339),
		RefreshToken:          refresh,
		RefreshTokenExpiresAt: refreshExp.UTC().Format(time.RFC3339),
		User:                  user,
	})
}

// Poll GET /api/auth/oauth/poll?device_id=xxx
//
// 桌面 / Android 客户端在打开系统浏览器后,持续 poll 这个端点拿 handoff code。
//
// 返回:
//   - 200 {"code": "..."}   —— 用户已在浏览器里完成登录,handoff 就绪。客户端
//     立刻 POST /api/auth/oauth/finalize 换 token。
//   - 204 No Content        —— 还没准备好,客户端继续 poll。
//   - 400                   —— 缺 device_id 参数。
//
// 客户端建议每 1.5~3s 轮询一次,总时长上限 5 分钟左右(超过就引导用户回登录页)。
// 出于反爬 / DoS,服务端不维护 long-poll,只做 stateless 的快速返回。
func (h *OAuthHandler) Poll(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "missing_device_id", "device_id required")
		return
	}
	code, ok := h.Pending.LookupDeviceHandoff(deviceID)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

// Done GET /api/auth/oauth/done
//
// 桌面 / Android 用户在系统浏览器里走完 OAuth 之后看到的最后一页。
// 不渲染 SPA 也不发任何 token —— 只展示"已登录,请回到客户端"。
//
// 收到 ?error=... 时把错误提示出来,便于排错。
func (h *OAuthHandler) Done(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	errCode := q.Get("error")
	errDesc := q.Get("error_description")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	body := successHTML
	if errCode != "" {
		// 简单 HTML 转义防 XSS;errCode/errDesc 都来自我方代码或 IdP,但稳一点
		body = strings.ReplaceAll(failureHTML, "{{ERR_CODE}}", htmlEscape(errCode))
		body = strings.ReplaceAll(body, "{{ERR_DESC}}", htmlEscape(errDesc))
	}
	_, _ = w.Write([]byte(body))
}

// AuthConfigPublic 返回前端登录页需要的认证配置(不包含任何 secret)。
type AuthConfigPublic struct {
	OAuthEnabled  bool   `json:"oauth_enabled"`
	OAuthProvider string `json:"oauth_provider,omitempty"`
	OAuthStartURL string `json:"oauth_start_url,omitempty"`
}

// Config GET /api/auth/config —— 前端登录页据此决定显示哪种登录形式。
func (h *OAuthHandler) Config(w http.ResponseWriter, r *http.Request) {
	resp := AuthConfigPublic{
		OAuthEnabled:  true,
		OAuthProvider: h.Provider.Name,
		OAuthStartURL: "/api/auth/oauth/start",
	}
	writeJSON(w, http.StatusOK, resp)
}

// redirectAfterCallback 按 ClientKind 选不同的回调展示路径。
//   - web:  302 到 ${frontend}/oauth/callback#code=... 或 #error=...
//   - desktop/android: 302 到 ${PUBLIC_BASE_URL}/api/auth/oauth/done(可带 ?error=)
//     桌面 / 移动客户端不直接用浏览器里的 SPA 完成 finalize,
//     handoff 已经通过 device_id 索引,客户端 poll 即可。
func (h *OAuthHandler) redirectAfterCallback(w http.ResponseWriter, r *http.Request, pending oauth.PendingState, handoff, errCode, errDesc string) {
	if pending.ClientKind == "desktop" || pending.ClientKind == "android" {
		target := h.doneURL()
		if errCode != "" {
			frag := url.Values{}
			frag.Set("error", errCode)
			if errDesc != "" {
				frag.Set("error_description", errDesc)
			}
			target = target + "?" + frag.Encode()
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	// web 走原有的"前端 fragment + finalize"流程
	target := h.Provider.FrontendCallbackURL()
	frag := url.Values{}
	if handoff != "" {
		frag.Set("code", handoff)
	}
	if errCode != "" {
		frag.Set("error", errCode)
		if errDesc != "" {
			frag.Set("error_description", errDesc)
		}
	}
	full := target + "#" + frag.Encode()
	http.Redirect(w, r, full, http.StatusFound)
}

// doneURL 返回桌面 / Android 用户在浏览器里看到的"登录成功"页面 URL。
// 直接复用 PUBLIC_BASE_URL,后端自己实现这个静态页(不进 SPA,避免水合 + 占用 token 槽)。
func (h *OAuthHandler) doneURL() string {
	// FrontendCallbackURL 推导自 RedirectURL 的同源,把 path 改成 /api/auth/oauth/done。
	u, err := url.Parse(h.Provider.FrontendCallbackURL())
	if err != nil {
		return "/api/auth/oauth/done"
	}
	u.Path = "/api/auth/oauth/done"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (h *OAuthHandler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

const successHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>TaskFlow · 登录成功</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; margin: 0; padding: 0;
         display: flex; align-items: center; justify-content: center; min-height: 100vh; background: #fafaff; color: #14142b; }
  @media (prefers-color-scheme: dark) { body { background: #0b0b14; color: #e8e8f5; } }
  .card { max-width: 420px; padding: 36px; border-radius: 16px; text-align: center;
          background: rgba(255,255,255,0.7); backdrop-filter: blur(8px); box-shadow: 0 8px 32px rgba(0,0,0,0.08); }
  @media (prefers-color-scheme: dark) { .card { background: rgba(20,20,32,0.7); box-shadow: 0 8px 32px rgba(0,0,0,0.4); } }
  .ok { width: 64px; height: 64px; border-radius: 50%; background: #22c55e; margin: 0 auto 16px;
        display: flex; align-items: center; justify-content: center; color: #fff; font-size: 32px; }
  h1 { margin: 0 0 8px; font-size: 22px; }
  p { margin: 8px 0; color: #5b5b7a; line-height: 1.6; }
  @media (prefers-color-scheme: dark) { p { color: #9c9cb4; } }
</style>
</head>
<body>
  <div class="card">
    <div class="ok">✓</div>
    <h1>登录成功</h1>
    <p>请回到 TaskFlow 桌面 / 移动客户端,客户端会在几秒内自动完成登录。</p>
    <p>本页可关闭。</p>
  </div>
</body>
</html>`

const failureHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>TaskFlow · 登录失败</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; margin: 0; padding: 0;
         display: flex; align-items: center; justify-content: center; min-height: 100vh; background: #fafaff; color: #14142b; }
  @media (prefers-color-scheme: dark) { body { background: #0b0b14; color: #e8e8f5; } }
  .card { max-width: 480px; padding: 36px; border-radius: 16px; text-align: center;
          background: rgba(255,255,255,0.7); backdrop-filter: blur(8px); box-shadow: 0 8px 32px rgba(0,0,0,0.08); }
  @media (prefers-color-scheme: dark) { .card { background: rgba(20,20,32,0.7); box-shadow: 0 8px 32px rgba(0,0,0,0.4); } }
  .x { width: 64px; height: 64px; border-radius: 50%; background: #ef4444; margin: 0 auto 16px;
       display: flex; align-items: center; justify-content: center; color: #fff; font-size: 32px; }
  h1 { margin: 0 0 8px; font-size: 22px; }
  p { margin: 8px 0; color: #5b5b7a; line-height: 1.6; }
  code { background: rgba(0,0,0,0.06); padding: 1px 6px; border-radius: 4px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  @media (prefers-color-scheme: dark) { p { color: #9c9cb4; } code { background: rgba(255,255,255,0.08); } }
</style>
</head>
<body>
  <div class="card">
    <div class="x">!</div>
    <h1>登录失败</h1>
    <p>错误码:<code>{{ERR_CODE}}</code></p>
    <p>{{ERR_DESC}}</p>
    <p>请回到客户端重试,或联系管理员。</p>
  </div>
</body>
</html>`

// DisabledLocalAuthHandler 给本地登录/注册端点用,在 OAuth 启用时返回 403。
func DisabledLocalAuthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "local_auth_disabled",
			"本地邮箱注册/登录已关闭,请通过认证中心登录")
	}
}

// AuthConfigDisabled 当 OAuth 未启用时,/api/auth/config 直接返回 oauth_enabled=false。
// 前端就退化到本地邮箱密码界面。
func AuthConfigDisabled(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, AuthConfigPublic{OAuthEnabled: false})
}

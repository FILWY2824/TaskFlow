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
// 三个端点:
//   - GET  /api/auth/oauth/start     —— 浏览器入口,302 到认证中心。
//   - GET  /api/auth/oauth/callback  —— 认证中心回调,完成 code 交换 + userinfo,
//     再 302 到前端 /oauth/callback#code=<handoff>。
//   - POST /api/auth/oauth/finalize  —— 前端把 handoff code 换本服务的 access/refresh JWT。
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

// Start GET /api/auth/oauth/start?device_id=xxx
//
// 生成 state + PKCE,302 到认证中心。device_id 可选,会原样带回到前端在 finalize
// 时用,以便发的 refresh token 与该设备绑定(便于"退出此设备")。
func (h *OAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	state, _, challenge, err := h.Pending.SaveState(deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	authURL := h.Provider.AuthorizeURLFor(state, challenge)
	// 注意:这里不能用 301 / 308,否则 token 端点的 POST 也会被某些浏览器/代理误处理。
	// 用 303(See Other)更精确,但 302 兼容性最好。本接口本来就是 GET,302 足够。
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback GET /api/auth/oauth/callback?code=&state=
//
// 失败一律 302 到前端 /oauth/callback?error=xxx,前端登录页会展示该错误。
// 不直接返回 JSON,因为这个端点是用户浏览器访问的,不是 fetch。
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// 认证中心可能直接报错(用户拒绝、配置错误等)。
	if e := q.Get("error"); e != "" {
		desc := q.Get("error_description")
		h.redirectFrontend(w, r, "", e, desc)
		return
	}

	state := q.Get("state")
	code := q.Get("code")
	if state == "" || code == "" {
		h.redirectFrontend(w, r, "", "invalid_request", "missing code or state")
		return
	}
	pending, ok := h.Pending.LookupState(state)
	if !ok {
		h.redirectFrontend(w, r, "", "invalid_state", "state expired or unknown — 请回到登录页重新发起")
		return
	}

	// 1) code -> token
	tokCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := h.Provider.Exchange(tokCtx, code, pending.CodeVerifier)
	if err != nil {
		h.logger().Warn("oauth token exchange failed", "err", err)
		h.redirectFrontend(w, r, "", "token_exchange_failed", err.Error())
		return
	}

	// 2) access_token -> userinfo
	uiCtx, cancel2 := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel2()
	info, err := h.Provider.UserInfo(uiCtx, tok.AccessToken)
	if err != nil {
		h.logger().Warn("oauth userinfo failed", "err", err)
		h.redirectFrontend(w, r, "", "userinfo_failed", err.Error())
		return
	}

	// 3) upsert 本地用户
	user, err := h.Users.UpsertOAuth(r.Context(), h.Provider.Name, info.Subject, info.Email, info.DisplayName)
	if err != nil {
		h.logger().Error("oauth upsert user", "err", err, "sub", info.Subject)
		h.redirectFrontend(w, r, "", "user_upsert_failed", err.Error())
		return
	}

	// 4) 给前端一个一次性 handoff code,前端立即拿它来换本服务的 JWT。
	handoff, err := h.Pending.SaveHandoff(user.ID, pending.DeviceID)
	if err != nil {
		h.redirectFrontend(w, r, "", "internal", err.Error())
		return
	}
	h.redirectFrontend(w, r, handoff, "", "")
}

// Finalize POST /api/auth/oauth/finalize  body: {"code": "..."}
//
// 用 handoff code 换本服务的 access + refresh token。前端在 /oauth/callback 拿到
// fragment 里的 code 后立刻调用本接口。
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

	// 复用 AuthHandler 的签发逻辑 —— 直接构造一个临时 AuthHandler 调 issueAndWrite
	// 会引入循环;干脆把签发流程内联在这里,与 auth.go 保持同形态。
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

// redirectFrontend 把用户重定向回前端 /oauth/callback,把 handoff_code 或 error
// 放在 URL fragment(#) 里 —— fragment 不会被发到服务端日志里,handoff 不外泄。
func (h *OAuthHandler) redirectFrontend(w http.ResponseWriter, r *http.Request, handoff, errCode, errDesc string) {
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

func (h *OAuthHandler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

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

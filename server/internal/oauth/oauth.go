// Package oauth 提供与外部 OAuth2 / OpenID Connect 认证中心交互的最小实现。
//
// 流程(Authorization Code + PKCE,RFC 6749 + RFC 7636):
//
//  1. 浏览器命中 /api/auth/oauth/start
//     -> 服务端生成 state(CSRF)和 code_verifier / code_challenge,
//     存到 PendingStore,302 到 authorize_url。
//  2. 用户在认证中心完成登录后,认证中心 302 到 redirect_url?code=XXX&state=YYY
//     -> 服务端 LookupState 校验 state,然后 Exchange(code, verifier) 调 token_url,
//     再 UserInfo(access_token) 调 userinfo_url。
//  3. 服务端用 (provider, sub) 在本库 Upsert 用户,签发本服务的 JWT。
//
// 这里不引入 golang.org/x/oauth2 这种重型依赖 —— 只是一次 POST 加一次 GET,
// 标准库就够,易审计、零额外攻击面。
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Provider 一个外部 OAuth2 提供方。所有字段取自 config.OAuthConfig。
type Provider struct {
	Name                string
	AuthorizeURL        string
	TokenURL            string
	UserInfoURL         string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	Scopes              []string
	FrontendRedirectURL string
	EmailField          string
	NameField           string
	SubjectField        string

	HTTPClient *http.Client
}

// NewProvider 用配置构造 Provider 并自带一个 10s 超时的 HTTP 客户端。
func NewProvider(name, authorizeURL, tokenURL, userInfoURL, clientID, clientSecret, redirectURL, frontendRedirectURL string, scopes []string, emailField, nameField, subjectField string) *Provider {
	return &Provider{
		Name:                name,
		AuthorizeURL:        authorizeURL,
		TokenURL:            tokenURL,
		UserInfoURL:         userInfoURL,
		ClientID:            clientID,
		ClientSecret:        clientSecret,
		RedirectURL:         redirectURL,
		Scopes:              scopes,
		FrontendRedirectURL: frontendRedirectURL,
		EmailField:          emailField,
		NameField:           nameField,
		SubjectField:        subjectField,
		HTTPClient:          &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthorizeURLFor 拼出把用户重定向到认证中心需要的 URL,使用 PKCE S256。
//
// 返回三个值:
//   - 完整的 authorize URL(直接 302 过去即可)
//   - state(已注册到 PendingStore,回调时用同一份 store 校验)
//   - code_verifier(已注册到 PendingStore,回调时用同一份 store 取出)
func (p *Provider) AuthorizeURLFor(state, codeChallenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", p.RedirectURL)
	q.Set("scope", strings.Join(p.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	sep := "?"
	if strings.Contains(p.AuthorizeURL, "?") {
		sep = "&"
	}
	return p.AuthorizeURL + sep + q.Encode()
}

// FrontendCallbackURL 服务端处理完 OAuth 后,重定向给前端的目标 URL(不含 fragment)。
//
// 优先使用配置 frontend_redirect_url;否则用 redirect_url 同源 + "/oauth/callback"。
func (p *Provider) FrontendCallbackURL() string {
	if p.FrontendRedirectURL != "" {
		return p.FrontendRedirectURL
	}
	u, err := url.Parse(p.RedirectURL)
	if err != nil {
		return "/oauth/callback"
	}
	u.Path = "/oauth/callback"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// TokenResponse 是 token_url 的标准响应(RFC 6749 §5.1)。
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// Exchange 用 authorization code 换 access token (RFC 6749 §4.1.3)。
//
// 同时把 client 凭证以 Basic auth 的形式带上(RFC 6749 §2.3.1 推荐方式),并兜底带在 form 里
// —— 部分实现只看 form,部分只看 header,两个都给最稳。
func (p *Provider) Exchange(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.RedirectURL)
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(p.ClientID, p.ClientSecret)

	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint %d: %s", res.StatusCode, truncate(string(body), 512))
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("token response missing access_token")
	}
	return &tr, nil
}

// UserInfo 用 access token 拉用户资料 (OIDC userinfo endpoint)。
//
// 解析时使用配置里的字段名(默认 sub / email / name),同时给若干常见的兼容回退。
type UserInfo struct {
	Subject     string
	Email       string
	DisplayName string
	Raw         map[string]any
}

func (p *Provider) UserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint %d: %s", res.StatusCode, truncate(string(body), 512))
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}
	// 部分实现把用户信息嵌套在 "user" / "data" 字段下,做一次浅展平。
	if inner, ok := raw["user"].(map[string]any); ok && len(inner) > 0 {
		raw = inner
	} else if inner, ok := raw["data"].(map[string]any); ok && len(inner) > 0 {
		raw = inner
	}
	info := &UserInfo{Raw: raw}
	info.Subject = pickString(raw, p.SubjectField, "sub", "id", "user_id", "uid")
	info.Email = pickString(raw, p.EmailField, "email", "mail")
	info.DisplayName = pickString(raw, p.NameField, "name", "preferred_username", "username", "nickname")
	if info.Subject == "" {
		return nil, errors.New("userinfo response missing subject (sub/id)")
	}
	return info, nil
}

// pickString 从 map 里按多个候选 key 找第一个非空字符串。数字会被自动转字符串。
func pickString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if k == "" {
			continue
		}
		v, ok := m[k]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case string:
			if x != "" {
				return x
			}
		case float64:
			return fmt.Sprintf("%.0f", x)
		case int64:
			return fmt.Sprintf("%d", x)
		case json.Number:
			return x.String()
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// =============================================================
// PendingStore: 内存里的短期登录态(state -> verifier)与
// 服务端到前端的 handoff(handoff_code -> userID)。
//
// 单实例进程足够,不涉及多副本部署 —— 上线前看一下流量再说。
// =============================================================

// PendingState 浏览器从 /oauth/start 跳到认证中心、又跳回 /oauth/callback 期间的临时状态。
type PendingState struct {
	State        string
	CodeVerifier string
	DeviceID     string
	CreatedAt    time.Time
}

// HandoffSession 服务端处理完回调后、前端来取本服务 access/refresh token 时用的一次性凭证。
type HandoffSession struct {
	UserID    int64
	DeviceID  string
	CreatedAt time.Time
}

// PendingStore 把 PendingState / HandoffSession 都放在内存里,定期清过期。
type PendingStore struct {
	mu         sync.Mutex
	states     map[string]PendingState
	handoffs   map[string]HandoffSession
	stateTTL   time.Duration
	handoffTTL time.Duration
}

// NewPendingStore stateTTL 建议 10 分钟(用户登录加授权页停留),handoffTTL 建议 60 秒
// (前端重定向回 /oauth/callback 后立刻发 finalize)。
func NewPendingStore(stateTTL, handoffTTL time.Duration) *PendingStore {
	return &PendingStore{
		states:     make(map[string]PendingState),
		handoffs:   make(map[string]HandoffSession),
		stateTTL:   stateTTL,
		handoffTTL: handoffTTL,
	}
}

// SaveState 生成新的 state + code_verifier + S256 challenge,写入 store,返回三元组。
func (s *PendingStore) SaveState(deviceID string) (state, verifier, challenge string, err error) {
	state, err = randomURLSafe(24)
	if err != nil {
		return "", "", "", err
	}
	verifier, err = randomURLSafe(64) // 43~128 字符 (RFC 7636 §4.1)
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])

	s.mu.Lock()
	s.states[state] = PendingState{
		State:        state,
		CodeVerifier: verifier,
		DeviceID:     deviceID,
		CreatedAt:    time.Now(),
	}
	s.mu.Unlock()
	return state, verifier, challenge, nil
}

// LookupState 取出并删除 state(一次性,防 replay)。过期返回 false。
func (s *PendingStore) LookupState(state string) (PendingState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.states[state]
	if !ok {
		return PendingState{}, false
	}
	delete(s.states, state)
	if time.Since(v.CreatedAt) > s.stateTTL {
		return PendingState{}, false
	}
	return v, true
}

// SaveHandoff 生成一次性 handoff code,关联到刚 upsert 出来的 userID。
func (s *PendingStore) SaveHandoff(userID int64, deviceID string) (string, error) {
	code, err := randomURLSafe(32)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.handoffs[code] = HandoffSession{
		UserID:    userID,
		DeviceID:  deviceID,
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()
	return code, nil
}

// LookupHandoff 取出并删除 handoff code(一次性)。过期返回 false。
func (s *PendingStore) LookupHandoff(code string) (HandoffSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.handoffs[code]
	if !ok {
		return HandoffSession{}, false
	}
	delete(s.handoffs, code)
	if time.Since(v.CreatedAt) > s.handoffTTL {
		return HandoffSession{}, false
	}
	return v, true
}

// GC 清理过期 state / handoff。建议每 5 分钟跑一次(由调用方启动 goroutine)。
func (s *PendingStore) GC() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.states {
		if now.Sub(v.CreatedAt) > s.stateTTL {
			delete(s.states, k)
		}
	}
	for k, v := range s.handoffs {
		if now.Sub(v.CreatedAt) > s.handoffTTL {
			delete(s.handoffs, k)
		}
	}
}

// randomURLSafe 生成 base64url 编码的随机字符串(byteLen 字节随机数 -> 约 1.33*byteLen 字符)。
func randomURLSafe(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashHex 仅用作日志/调试时打印,避免把原始 token 落盘。
func HashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

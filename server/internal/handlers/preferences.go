package handlers

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/youruser/taskflow/internal/middleware"
	"github.com/youruser/taskflow/internal/store"
)

// PreferencesHandler 暴露 /api/me/preferences,负责跨端用户偏好读写。
//
// 路由(均需要认证):
//
//	GET    /api/me/preferences            ?scope=web|android|windows|common (可选,空=全部)
//	PUT    /api/me/preferences            body: { items: [{scope,key,value}, ...] } 批量
//	PUT    /api/me/preferences/{scope}/{key}  body: { value: "..." }                    单条
//	DELETE /api/me/preferences/{scope}/{key}                                            单条
//
// 设计要点:
//   - 服务端只做透明键值存储 + 合法性校验,不解释 value 的语义;
//     客户端定义自己的键空间(例如 android.notification.fullscreen / web.in_app_toast)。
//   - scope 集合是闭集合,不允许任意字符串,防止客户端误把生产数据塞进偏好表。
//   - key 必须由 [a-z0-9._-] 组成,1~64 字符,防止 SQL 注入边界 + URL path 兼容。
//   - value 上限 4 KB(UTF-8),超过返回 400。
type PreferencesHandler struct {
	Prefs *store.PreferenceStore
}

func NewPreferencesHandler(p *store.PreferenceStore) *PreferencesHandler {
	return &PreferencesHandler{Prefs: p}
}

const (
	maxPrefKeyLen   = 64
	maxPrefValueLen = 4 * 1024
)

// preferenceDTO 是 GET / PUT 单条的统一返回形状。
type preferenceDTO struct {
	Scope     string `json:"scope"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt string `json:"updated_at"`
}

func toDTO(p *store.Preference) preferenceDTO {
	return preferenceDTO{
		Scope:     p.Scope,
		Key:       p.Key,
		Value:     p.Value,
		UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List GET /api/me/preferences[?scope=...]
func (h *PreferencesHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope != "" && !store.IsAllowedScope(scope) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid scope")
		return
	}
	items, err := h.Prefs.ListByScope(r.Context(), uid, scope)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := make([]preferenceDTO, 0, len(items))
	for i := range items {
		out = append(out, toDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type putOneRequest struct {
	Value string `json:"value"`
}

// PutOne PUT /api/me/preferences/{scope}/{key}
func (h *PreferencesHandler) PutOne(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	scope := strings.ToLower(r.PathValue("scope"))
	key := r.PathValue("key")
	if !store.IsAllowedScope(scope) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid scope")
		return
	}
	if msg := validatePrefKey(key); msg != "" {
		writeError(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	var req putOneRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if utf8.RuneCountInString(req.Value) > maxPrefValueLen {
		writeError(w, http.StatusBadRequest, "bad_request", "value too long")
		return
	}
	p, err := h.Prefs.Upsert(r.Context(), uid, scope, key, req.Value)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(p))
}

type bulkRequest struct {
	Items []preferenceDTO `json:"items"`
}

// PutBulk PUT /api/me/preferences  —— 一次性写多条。
//
// 客户端可以在登录后或在"恢复默认"后调用一次,把整个本地视图推上去。
// 该接口对每条做与 PutOne 同样的校验,任一不合法即整次拒绝(不会半写)。
func (h *PreferencesHandler) PutBulk(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var req bulkRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []preferenceDTO{}})
		return
	}
	if len(req.Items) > 200 {
		writeError(w, http.StatusBadRequest, "bad_request", "too many items (max 200)")
		return
	}

	prepared := make([]store.Preference, 0, len(req.Items))
	for _, it := range req.Items {
		scope := strings.ToLower(strings.TrimSpace(it.Scope))
		if !store.IsAllowedScope(scope) {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid scope: "+it.Scope)
			return
		}
		if msg := validatePrefKey(it.Key); msg != "" {
			writeError(w, http.StatusBadRequest, "bad_request", msg+": "+it.Key)
			return
		}
		if utf8.RuneCountInString(it.Value) > maxPrefValueLen {
			writeError(w, http.StatusBadRequest, "bad_request", "value too long for "+it.Key)
			return
		}
		prepared = append(prepared, store.Preference{Scope: scope, Key: it.Key, Value: it.Value})
	}
	if err := h.Prefs.BulkUpsert(r.Context(), uid, prepared); err != nil {
		writeStoreError(w, err)
		return
	}
	// 写完再读一次,把 updated_at 一并返回,客户端可以拿到来更新本地缓存的时间戳。
	items, err := h.Prefs.ListByScope(r.Context(), uid, "")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := make([]preferenceDTO, 0, len(items))
	for i := range items {
		out = append(out, toDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// DeleteOne DELETE /api/me/preferences/{scope}/{key}
func (h *PreferencesHandler) DeleteOne(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	scope := strings.ToLower(r.PathValue("scope"))
	key := r.PathValue("key")
	if !store.IsAllowedScope(scope) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid scope")
		return
	}
	if msg := validatePrefKey(key); msg != "" {
		writeError(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	if err := h.Prefs.Delete(r.Context(), uid, scope, key); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validatePrefKey 校验 key 字符集与长度,通过返回 ""。
//
// key 命名约定(由客户端遵守,服务端只做最小必要的字符校验):
//
//	notification.in_app_toast
//	notification.full_screen_alarm
//	pomodoro.auto_complete
//	ui.theme
func validatePrefKey(k string) string {
	if k == "" {
		return "key required"
	}
	if utf8.RuneCountInString(k) > maxPrefKeyLen {
		return "key too long"
	}
	for _, c := range k {
		ok := (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-'
		if !ok {
			return "key must match [a-z0-9._-]"
		}
	}
	return ""
}

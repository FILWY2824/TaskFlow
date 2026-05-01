package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/middleware"
	"github.com/youruser/taskflow/internal/store"
	"github.com/youruser/taskflow/internal/telegram"
)

// TelegramHandler Phase 4 的 HTTP 端点集合。
type TelegramHandler struct {
	Store         *store.TelegramStore
	Bot           *telegram.Client
	BotUsername   string // 例如 "TaskFlowBot",用于在响应中给客户端拼 deep-link
	WebhookSecret string // setWebhook 时设置的 secret_token,Telegram 会通过 X-Telegram-Bot-Api-Secret-Token 回传
	BindTokenTTL  time.Duration
	Logger        *slog.Logger
}

// NewTelegramHandler 构造。BindTokenTTL 传 0 表示用默认 10 分钟。
func NewTelegramHandler(s *store.TelegramStore, bot *telegram.Client, botUsername, webhookSecret string,
	bindTokenTTL time.Duration, logger *slog.Logger) *TelegramHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &TelegramHandler{
		Store:         s,
		Bot:           bot,
		BotUsername:   botUsername,
		WebhookSecret: webhookSecret,
		BindTokenTTL:  bindTokenTTL,
		Logger:        logger,
	}
}

type bindTokenResponse struct {
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
	BotUsername string    `json:"bot_username"`
	DeepLinkWeb string    `json:"deep_link_web"` // https://t.me/<bot>?start=bind_<token>
	DeepLinkApp string    `json:"deep_link_app"` // tg://resolve?domain=<bot>&start=bind_<token>
}

// configResponse 给前端用于探测服务端是否启用了 Telegram 集成。
type configResponse struct {
	Enabled     bool   `json:"enabled"`
	BotUsername string `json:"bot_username"`
}

// GetConfig GET /api/telegram/config
//
// 客户端在打开 Telegram 绑定页时调用,用来判断:
//   - 服务端是否真的开启了 Telegram 集成(bot_token + bot_username 都有)
//   - bot_username 是什么(用来在 UI 里展示 "@xxx_bot",或拼 deep link 兜底)
//
// 不开启 Telegram 的部署也能正常用 App,只是绑定页会显示"管理员未启用"。
func (h *TelegramHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	resp := configResponse{
		Enabled:     h.Bot != nil && h.Bot.Enabled() && h.BotUsername != "",
		BotUsername: h.BotUsername,
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateBindToken POST /api/telegram/bind-token
//
// 让登录用户获取一次性 bind_token,并附带可直接打开 Telegram 的 deep link。
//
// 客户端流程:
//  1. 调用此接口拿 token + deep_link;
//  2. 在浏览器/手机上打开 deep_link,用户在 Telegram 客户端按 Start;
//  3. 客户端轮询 /api/telegram/bind-status?token=... 直到 status=bound。
func (h *TelegramHandler) CreateBindToken(w http.ResponseWriter, r *http.Request) {
	// 先校验:服务端没配 bot,生成 token 也是无意义的(deep link 会拼成 "https://t.me/?start=..."),
	// 直接给前端一个明确错误,前端可据此渲染"管理员未配置"提示。
	if h.Bot == nil || !h.Bot.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "telegram_disabled",
			"服务端未配置 Telegram 机器人(bot_token 为空)。请联系管理员在 config.toml 的 [telegram] 段填好 bot_token / bot_username / webhook_secret 后重启服务。")
		return
	}
	if h.BotUsername == "" {
		writeError(w, http.StatusServiceUnavailable, "telegram_disabled",
			"服务端配置了 bot_token 但缺少 bot_username,无法生成绑定链接。请联系管理员补全配置。")
		return
	}
	uid := middleware.UserIDFrom(r.Context())
	bt, err := h.Store.CreateBindToken(r.Context(), uid, h.BindTokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	resp := bindTokenResponse{
		Token:       bt.Token,
		ExpiresAt:   bt.ExpiresAt,
		BotUsername: h.BotUsername,
		DeepLinkWeb: fmt.Sprintf("https://t.me/%s?start=%s%s", h.BotUsername, telegram.BindPayloadPrefix, bt.Token),
		DeepLinkApp: fmt.Sprintf("tg://resolve?domain=%s&start=%s%s", h.BotUsername, telegram.BindPayloadPrefix, bt.Token),
	}
	writeJSON(w, http.StatusCreated, resp)
}

type bindStatusResponse struct {
	Status    string         `json:"status"` // pending | expired | bound | not_found
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
	Binding   *store.Binding `json:"binding,omitempty"`
}

// GetBindStatus GET /api/telegram/bind-status?token=...
//
// 客户端轮询用。安全考虑:只让 token 的所有者能看到结果——
// token 本身就是高熵字符串,在传输中 == "拥有它就是合法查询",但我们额外要求请求带登录态,
// 且 user_id 必须匹配 token 上记录的 user_id。
func (h *TelegramHandler) GetBindStatus(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "token required")
		return
	}
	bt, err := h.Store.LookupBindToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, bindStatusResponse{Status: "not_found"})
			return
		}
		writeStoreError(w, err)
		return
	}
	if bt.UserID != uid {
		// 别人的 token 不让你看,直接装作不存在(防枚举)
		writeJSON(w, http.StatusOK, bindStatusResponse{Status: "not_found"})
		return
	}
	now := time.Now().UTC()
	resp := bindStatusResponse{ExpiresAt: &bt.ExpiresAt}
	switch {
	case bt.UsedAt != nil:
		// 历史上这里返回 "used"，但前端期望的字段是 "bound"。统一为 "bound"。
		resp.Status = "bound"
		// 顺带把当前用户的最新一条 binding 带回去,方便前端立刻渲染。
		bindings, _ := h.Store.ListBindings(r.Context(), uid)
		if n := len(bindings); n > 0 {
			resp.Binding = bindings[n-1]
		}
	case !bt.ExpiresAt.After(now):
		resp.Status = "expired"
	default:
		resp.Status = "pending"
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListBindings GET /api/telegram/bindings
func (h *TelegramHandler) ListBindings(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	items, err := h.Store.ListBindings(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*store.Binding{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Unbind POST /api/telegram/unbind  body: {"id": <binding_id>}
func (h *TelegramHandler) Unbind(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var body struct {
		ID int64 `json:"id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.ID <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	if err := h.Store.DeleteBinding(r.Context(), uid, body.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SendTest POST /api/telegram/test  body: {"binding_id": <id>}
//
// 测试发送一条 "TaskFlow 测试消息" 给该 binding。
func (h *TelegramHandler) SendTest(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	if !h.Bot.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "telegram_disabled", "telegram bot_token not configured on server")
		return
	}
	var body struct {
		BindingID int64 `json:"binding_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	bindings, err := h.Store.ListBindings(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var target *store.Binding
	for _, b := range bindings {
		if b.ID == body.BindingID {
			target = b
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "not_found", "binding not found")
		return
	}
	if !target.IsEnabled {
		writeError(w, http.StatusBadRequest, "binding_disabled", "binding is disabled")
		return
	}
	msg := "✅ TaskFlow 已成功连接到这个聊天。今后到点的提醒会发到这里。"
	if _, err := h.Bot.SendMessage(r.Context(), target.ChatID, msg, telegram.SendMessageOptions{}); err != nil {
		writeError(w, http.StatusBadGateway, "telegram_send_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Webhook POST /api/telegram/webhook
//
// Telegram 在我们调用过 setWebhook 之后,会把所有更新 POST 到这里。
//
// 安全:
//   - 必须验证 X-Telegram-Bot-Api-Secret-Token 与配置的 webhook_secret 匹配,
//     否则任何人都能伪造 update。
//   - 如果服务器没配 webhook_secret,我们直接 401 拒绝(配置错误)。
//
// 内容:
//   - 我们只关心 message.text 是 /start <payload> 的更新。
//   - 其他更新一律 200 ok 静默丢弃 —— 对 Telegram 而言只要我们 200 它就不重试。
func (h *TelegramHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	if h.WebhookSecret == "" {
		// 服务器没配 secret 时拒绝,避免裸暴露 webhook。
		writeError(w, http.StatusUnauthorized, "webhook_secret_unset", "server does not have a webhook secret")
		return
	}
	got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.WebhookSecret)) != 1 {
		writeError(w, http.StatusUnauthorized, "bad_secret", "invalid webhook secret")
		return
	}

	// Telegram 默认更新 size 大概几 KB,我们留 256KB 余量。
	r.Body = http.MaxBytesReader(nil, r.Body, 256*1024)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "read body: "+err.Error())
		return
	}
	var upd telegram.Update
	if err := json.Unmarshal(raw, &upd); err != nil {
		// 内容就坏了,不重试
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}

	msg := upd.Message
	if msg == nil {
		msg = upd.EditedMessage
	}
	if msg == nil || msg.Chat.ID == 0 {
		// 不是消息更新,忽略。Telegram 看到 200 就不会重试。
		w.WriteHeader(http.StatusOK)
		return
	}

	payload, isStart := telegram.ParseStartCommand(msg.Text)
	if !isStart {
		// 非 /start 命令我们不处理。返回 200 防止重投递。
		// 后续可以扩展回复类似 "请前往 App 创建任务" 的提示。
		w.WriteHeader(http.StatusOK)
		return
	}

	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	username := msg.From.Username

	tok, ok := telegram.ExtractBindToken(payload)
	if !ok {
		// /start 但没带正确 payload。回复一句友好提示。
		h.replyAndIgnoreErr(r.Context(), chatID, "👋 你好。请回到 TaskFlow App,点击\"绑定 Telegram\"按钮,从那里跳进来。")
		w.WriteHeader(http.StatusOK)
		return
	}

	binding, err := h.Store.ConsumeBindToken(r.Context(), tok, chatID, username)
	if err != nil {
		// 绑定失败:告诉用户,但仍然回 200(否则 Telegram 会一直重投同一 update)。
		h.Logger.Warn("consume bind token failed",
			"err", err,
			"chat_id", chatID,
			"token_prefix", safeTokenPrefix(tok))
		var hint string
		switch {
		case errors.Is(err, store.ErrNotFound):
			hint = "❌ 链接无效。请回到 App 重新生成绑定链接。"
		case errors.Is(err, store.ErrConflict):
			hint = "❌ 此 Telegram 账号已绑定到另一个用户。先在原账号解绑后再试。"
		case strings.Contains(err.Error(), "expired"):
			hint = "⌛️ 链接已过期。请回到 App 重新生成。"
		case strings.Contains(err.Error(), "already used"):
			hint = "ℹ️ 这个绑定链接已经被使用过。如需重新绑定请回到 App 重新生成。"
		default:
			hint = "❌ 绑定失败,请稍后重试。"
		}
		h.replyAndIgnoreErr(r.Context(), chatID, hint)
		w.WriteHeader(http.StatusOK)
		return
	}

	// 成功:回复用户,注意此时 binding != nil。
	h.replyAndIgnoreErr(r.Context(), chatID,
		fmt.Sprintf("✅ 绑定成功!从现在起,TaskFlow 提醒会推送到这里(用户 ID #%d)。", binding.UserID))

	w.WriteHeader(http.StatusOK)
}

func (h *TelegramHandler) replyAndIgnoreErr(ctx context.Context, chatID, text string) {
	if !h.Bot.Enabled() {
		return
	}
	if _, err := h.Bot.SendMessage(ctx, chatID, text, telegram.SendMessageOptions{}); err != nil {
		h.Logger.Warn("telegram reply failed", "err", err, "chat_id", chatID)
	}
}

// safeTokenPrefix 给日志用。完整 token 视为半秘密,只打印前 4 字符。
func safeTokenPrefix(tok string) string {
	if len(tok) <= 4 {
		return tok
	}
	return tok[:4] + "..."
}

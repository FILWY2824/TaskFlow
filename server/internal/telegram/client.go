// Package telegram 是一个最小化的 Telegram Bot API 客户端封装。
//
// 设计原则:
//   - 只暴露我们用得上的两个方法:SendMessage 与 SetWebhook。
//   - 不引入 go-telegram-bot-api 等大依赖(MVP 不需要,且要保持 VPS 内存占用低)。
//   - 所有调用都允许传入 ctx,便于优雅退出。
package telegram

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
	"time"
)

// Client 一个 Bot API 客户端。零值不可用,请用 NewClient。
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// NewClient 创建客户端。token 为 BotFather 颁发的 bot_token。
// baseURL 留空则使用默认 https://api.telegram.org。
func NewClient(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &Client{
		token:   token,
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Enabled 返回 token 是否已配置。未配置时调用方应跳过推送。
func (c *Client) Enabled() bool { return c != nil && strings.TrimSpace(c.token) != "" }

// apiResponse 是 Bot API 的统一响应包络。
type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// Message 极简的 Telegram 消息形状(只要我们用到的字段)。
type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Date      int64  `json:"date"`
	Text      string `json:"text"`
	From      User   `json:"from"`
}

// Chat 极简的聊天形状。
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// User Telegram 用户。
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// Update Telegram webhook 推送的更新对象。我们只关心 message 字段。
type Update struct {
	UpdateID      int64    `json:"update_id"`
	Message       *Message `json:"message,omitempty"`
	EditedMessage *Message `json:"edited_message,omitempty"`
}

// SendMessageOptions 调用 sendMessage 的可选参数。
type SendMessageOptions struct {
	ParseMode             string // "HTML" / "MarkdownV2" / ""
	DisableNotification   bool
	DisableWebPagePreview bool
}

// SendMessage 给指定 chat_id 发送一条文本消息。
// 即使 token 未配置也不会 panic,会直接返回错误。
func (c *Client) SendMessage(ctx context.Context, chatID, text string, opts SendMessageOptions) (*Message, error) {
	if !c.Enabled() {
		return nil, errors.New("telegram client disabled (bot_token not set)")
	}
	if strings.TrimSpace(chatID) == "" {
		return nil, errors.New("chat_id required")
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text required")
	}

	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if opts.ParseMode != "" {
		body["parse_mode"] = opts.ParseMode
	}
	if opts.DisableNotification {
		body["disable_notification"] = true
	}
	if opts.DisableWebPagePreview {
		body["disable_web_page_preview"] = true
	}

	var msg Message
	if err := c.call(ctx, "sendMessage", body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SetWebhookOptions 调用 setWebhook 的参数。
type SetWebhookOptions struct {
	URL            string
	SecretToken    string // 写入 X-Telegram-Bot-Api-Secret-Token header
	MaxConnections int    // 1..100,默认 40
	AllowedUpdates []string
	DropPending    bool
}

// SetWebhook 注册 webhook URL。
// 一般在部署时一次性调用即可,不放在每次启动里跑。
func (c *Client) SetWebhook(ctx context.Context, opt SetWebhookOptions) error {
	if !c.Enabled() {
		return errors.New("telegram client disabled (bot_token not set)")
	}
	if strings.TrimSpace(opt.URL) == "" {
		return errors.New("url required")
	}
	body := map[string]any{
		"url": opt.URL,
	}
	if opt.SecretToken != "" {
		body["secret_token"] = opt.SecretToken
	}
	if opt.MaxConnections > 0 {
		body["max_connections"] = opt.MaxConnections
	}
	if len(opt.AllowedUpdates) > 0 {
		body["allowed_updates"] = opt.AllowedUpdates
	}
	if opt.DropPending {
		body["drop_pending_updates"] = true
	}
	var ok bool
	return c.call(ctx, "setWebhook", body, &ok)
}

// DeleteWebhook 删除当前 webhook,常用于切换部署时。
func (c *Client) DeleteWebhook(ctx context.Context) error {
	if !c.Enabled() {
		return errors.New("telegram client disabled (bot_token not set)")
	}
	var ok bool
	return c.call(ctx, "deleteWebhook", map[string]any{}, &ok)
}

// call 执行一次 Bot API 调用。method 形如 "sendMessage"。
// 把 result 字段反序列化到 dst(传 nil 表示不需要 result)。
func (c *Client) call(ctx context.Context, method string, payload any, dst any) error {
	endpoint := c.baseURL + "/bot" + url.PathEscape(c.token) + "/" + method
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram %s marshal: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("telegram %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s send: %w", method, err)
	}
	defer resp.Body.Close()

	// 限制读取,防止恶意/异常返回吃光内存。
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("telegram %s read: %w", method, err)
	}

	var env apiResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("telegram %s decode (status=%d, body=%q): %w", method, resp.StatusCode, truncate(string(raw), 200), err)
	}
	if !env.OK {
		return fmt.Errorf("telegram %s api error (code=%d): %s", method, env.ErrorCode, env.Description)
	}
	if dst == nil || len(env.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Result, dst); err != nil {
		return fmt.Errorf("telegram %s decode result: %w", method, err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

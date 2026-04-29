package telegram

import (
	"strings"
)

// BindPayloadPrefix 是我们使用的 deep-link 启动负载前缀。
//
// 用户点开 https://t.me/YourBot?start=bind_<token> 时,
// Telegram 会把 "/start bind_<token>" 作为消息文本送到 webhook。
// 我们提取 <token>,在服务端用它兑换一个 user_id -> chat_id 的绑定。
const BindPayloadPrefix = "bind_"

// ParseStartCommand 从消息文本中解析 /start 后面的 payload。
//
// 接受的形式:
//
//	"/start bind_abc123"
//	"/start@YourBot bind_abc123"  (群里 @-bot 形式;隐私聊天里很少出现,但兼容一下)
//
// 第二个返回值表示这是否是一个 /start 命令(不一定带 payload)。
func ParseStartCommand(text string) (payload string, isStart bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", false
	}
	// 第一段是命令本身
	parts := strings.SplitN(t, " ", 2)
	cmd := parts[0]
	// 去掉 @bot 后缀
	if i := strings.Index(cmd, "@"); i > 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/start") {
		return "", false
	}
	if len(parts) < 2 {
		return "", true
	}
	return strings.TrimSpace(parts[1]), true
}

// ExtractBindToken 取出形如 "bind_xxx" 的 payload 中的 token,
// 不是该前缀则返回 ""(同时返回 false)。
func ExtractBindToken(payload string) (string, bool) {
	if !strings.HasPrefix(payload, BindPayloadPrefix) {
		return "", false
	}
	tok := strings.TrimPrefix(payload, BindPayloadPrefix)
	if tok == "" {
		return "", false
	}
	return tok, true
}

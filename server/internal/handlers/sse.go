package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/youruser/taskflow/internal/events"
	"github.com/youruser/taskflow/internal/middleware"
)

// SSEHandler 提供 GET /ws/events:Server-Sent Events 流。
//
// 协议:
//   - Content-Type: text/event-stream
//   - 每条事件一段 "data: {...}\n\n" JSON
//   - 每 25s 发一次 ":\n\n" 心跳防止反向代理 / 中间盒断流
//
// 与 sync_events 的关系:
//   - SSE 推送是"低延迟提示",不替代 /api/sync/pull。客户端断线重连后应当先 pull
//     一次,再开新的 SSE。
type SSEHandler struct {
	Hub *events.Hub
}

func NewSSEHandler(hub *events.Hub) *SSEHandler {
	return &SSEHandler{Hub: hub}
}

func (h *SSEHandler) Stream(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_flusher",
			"streaming not supported by underlying writer")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 关闭 nginx 等代理的缓冲(nginx 看到这个 header 才会立刻 flush)
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sub := h.Hub.Subscribe(uid)
	defer sub.Close()

	// 立即 flush 头,客户端 onopen 触发更快。
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	enc := json.NewEncoder(w)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-sub.C:
			if !ok {
				// hub 关了
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: ", safeEventName(ev.Type)); err != nil {
				return
			}
			if err := enc.Encode(ev); err != nil {
				return
			}
			// json.Encoder.Encode 已经在尾巴加了 "\n",SSE 还需要再一个空行
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// safeEventName 防止恶意 type 把 SSE 帧切坏。允许的字符:字母、数字、下划线、短横线。
func safeEventName(t string) string {
	if t == "" {
		return "message"
	}
	out := make([]byte, 0, len(t))
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c == '-' || c == '_' || (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "message"
	}
	return string(out)
}

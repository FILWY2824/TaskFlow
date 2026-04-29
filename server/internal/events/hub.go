// Package events 提供一个进程内的、按 user_id 分组的事件总线,
// 配合 Server-Sent Events (SSE) 把服务端调度器投递的通知实时推到在线客户端。
//
// 设计:
//   - 不引入第三方包,零依赖。
//   - 每个用户对应一组订阅 channel(同一用户可在多个端登录)。
//   - 发布是非阻塞的:订阅方处理慢就丢消息(每 channel 32 缓冲),宁可漏推也不阻塞调度器。
//     反正客户端最终会通过 /api/sync/pull 兜底拉到 sync_event,SSE 只是"提示更早一点"。
//   - 关闭单个订阅是幂等的。
package events

import (
	"sync"
	"sync/atomic"
)

// Event 通过 SSE 推给客户端的事件。Type 当前只有 "notification" 一种,
// 但保留扩展位,后续可加 "todo_changed" 等。
//
// 这里特意不直接把 *Notification 放进去,因为 events 包应当对 store 包一无所知;
// 字段平铺为 JSON-friendly 结构,handlers/sse.go 直接 json.Encode。
type Event struct {
	Type           string `json:"type"`
	NotificationID int64  `json:"notification_id,omitempty"`
	ReminderRuleID int64  `json:"reminder_rule_id,omitempty"`
	TodoID         int64  `json:"todo_id,omitempty"`
	Title          string `json:"title,omitempty"`
	Body           string `json:"body,omitempty"`
	FireAtUnix     int64  `json:"fire_at_unix,omitempty"`
}

// Hub 进程内事件总线。零值不可用,请用 NewHub。
type Hub struct {
	mu     sync.RWMutex
	users  map[int64]map[*Subscription]struct{} // user_id -> subscription set
	closed atomic.Bool
}

func NewHub() *Hub {
	return &Hub{users: make(map[int64]map[*Subscription]struct{})}
}

// Subscription 一个具体客户端的订阅句柄。
type Subscription struct {
	UserID int64
	C      chan Event
	hub    *Hub
	once   sync.Once
}

// Subscribe 让 SSE handler 拿到一个只读 channel。Close() 必须被调用以释放资源。
//
// 同一 user 多次 Subscribe 互不干扰,各拿各的 channel。
func (h *Hub) Subscribe(userID int64) *Subscription {
	sub := &Subscription{
		UserID: userID,
		C:      make(chan Event, 32),
		hub:    h,
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed.Load() {
		// 已关闭就给一个空且立刻关闭的 channel
		close(sub.C)
		return sub
	}
	set, ok := h.users[userID]
	if !ok {
		set = make(map[*Subscription]struct{})
		h.users[userID] = set
	}
	set[sub] = struct{}{}
	return sub
}

// Close 退订并关闭 channel。多次调用安全。
func (s *Subscription) Close() {
	s.once.Do(func() {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
		set, ok := s.hub.users[s.UserID]
		if ok {
			delete(set, s)
			if len(set) == 0 {
				delete(s.hub.users, s.UserID)
			}
		}
		close(s.C)
	})
}

// Publish 把一个事件投递给某用户所有订阅。非阻塞:对每个订阅做 non-blocking send,
// 缓冲满则丢这一条(下条还能进)。
func (h *Hub) Publish(userID int64, ev Event) {
	if h.closed.Load() {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.users[userID]
	if !ok {
		return
	}
	for sub := range set {
		select {
		case sub.C <- ev:
		default:
			// 缓冲已满,丢弃。客户端通过 sync 兜底。
		}
	}
}

// Shutdown 关闭所有订阅。Publish 之后调用是安全的(已关闭则 no-op)。
func (h *Hub) Shutdown() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.users {
		for sub := range set {
			// 不调用 sub.Close 以避免回锁,直接关 channel。
			// 这里 once 没走过,后续 sub.Close() 不会重复 close。
			sub.once.Do(func() { close(sub.C) })
		}
	}
	h.users = map[int64]map[*Subscription]struct{}{}
}

// CountSubscribers 仅用于调试 / metrics。
func (h *Hub) CountSubscribers(userID int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID])
}

package handlers

import (
	"net/http"
	"strconv"

	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/models"
	"github.com/youruser/todoalarm/internal/store"
)

type SyncHandler struct {
	Events *store.SyncEventStore
}

func NewSyncHandler(s *store.SyncEventStore) *SyncHandler {
	return &SyncHandler{Events: s}
}

// Pull GET /api/sync/pull?since=<cursor>&limit=<n>
//
// 返回:
//
//	{
//	  "events": [...],         // 自 since 之后(不含)的事件,按 id 升序
//	  "next_cursor": <int>,    // 客户端下次传给 since 的值
//	  "has_more": <bool>       // 是否还有更多事件需要继续拉取
//	}
//
// 首次同步可不传 since(默认 0),拿到 next_cursor 后保存即可。
func (h *SyncHandler) Pull(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	q := r.URL.Query()

	var since int64
	if v := q.Get("since"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid since")
			return
		}
		since = n
	}
	limit := 500
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid limit")
			return
		}
		if n > 1000 {
			n = 1000
		}
		limit = n
	}

	events, err := h.Events.Pull(r.Context(), uid, since, limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if events == nil {
		events = []*models.SyncEvent{}
	}

	nextCursor := since
	hasMore := false
	if len(events) > 0 {
		nextCursor = events[len(events)-1].ID
		// 如果取到了 limit 条,可能还有更多
		hasMore = len(events) >= limit
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":      events,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// Cursor GET /api/sync/cursor
// 返回当前用户最新事件 id,客户端首次启动可作为基线。
func (h *SyncHandler) Cursor(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	cur, err := h.Events.LatestCursor(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cur})
}

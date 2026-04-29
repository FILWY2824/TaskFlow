package handlers

import (
	"net/http"

	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/store"
)

type NotificationsHandler struct {
	Store *store.NotificationStore
}

func NewNotificationsHandler(s *store.NotificationStore) *NotificationsHandler {
	return &NotificationsHandler{Store: s}
}

// Index GET /api/notifications?only_unread=&limit=&offset=
func (h *NotificationsHandler) Index(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	q := r.URL.Query()

	f := store.NotificationFilter{}
	if v := q.Get("only_unread"); v == "true" || v == "1" {
		f.OnlyUnread = true
	}
	if v := q.Get("limit"); v != "" {
		if n, err := parseInt64(v); err == nil {
			f.Limit = int(n)
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := parseInt64(v); err == nil {
			f.Offset = int(n)
		}
	}

	items, err := h.Store.List(r.Context(), uid, f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*store.Notification{}
	}
	unread, err := h.Store.UnreadCount(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"unread_count": unread,
	})
}

// Show GET /api/notifications/{id}
func (h *NotificationsHandler) Show(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	n, err := h.Store.Get(r.Context(), uid, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// MarkRead POST /api/notifications/{id}/read
func (h *NotificationsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Store.MarkRead(r.Context(), uid, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllRead POST /api/notifications/read-all
func (h *NotificationsHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	n, err := h.Store.MarkAllRead(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

// UnreadCount GET /api/notifications/unread-count
func (h *NotificationsHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	n, err := h.Store.UnreadCount(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread_count": n})
}

package handlers

import (
	"net/http"
	"strings"

	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/models"
	"github.com/youruser/todoalarm/internal/store"
)

type ListsHandler struct {
	Store *store.ListStore
}

func NewListsHandler(s *store.ListStore) *ListsHandler { return &ListsHandler{Store: s} }

type listRequest struct {
	Name       string `json:"name"`
	Color      string `json:"color"`
	Icon       string `json:"icon"`
	SortOrder  int    `json:"sort_order"`
	IsDefault  bool   `json:"is_default"`
	IsArchived bool   `json:"is_archived"`
}

func (h *ListsHandler) Index(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	items, err := h.Store.List(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*models.List{} // 保证 JSON 输出为 [] 而非 null
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ListsHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var req listRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name required")
		return
	}
	if len(req.Name) > 200 {
		writeError(w, http.StatusBadRequest, "bad_request", "name too long")
		return
	}
	out, err := h.Store.Create(r.Context(), uid, store.ListInput{
		Name: req.Name, Color: req.Color, Icon: req.Icon,
		SortOrder: req.SortOrder, IsDefault: req.IsDefault, IsArchived: req.IsArchived,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *ListsHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req listRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name required")
		return
	}
	out, err := h.Store.Update(r.Context(), uid, id, store.ListInput{
		Name: req.Name, Color: req.Color, Icon: req.Icon,
		SortOrder: req.SortOrder, IsDefault: req.IsDefault, IsArchived: req.IsArchived,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ListsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Store.Delete(r.Context(), uid, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

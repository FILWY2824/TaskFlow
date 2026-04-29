package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/models"
	"github.com/youruser/todoalarm/internal/store"
)

type SubtasksHandler struct {
	Subtasks *store.SubtaskStore
}

func NewSubtasksHandler(s *store.SubtaskStore) *SubtasksHandler {
	return &SubtasksHandler{Subtasks: s}
}

type subtaskRequest struct {
	Title     string `json:"title"`
	SortOrder int    `json:"sort_order"`
}

func (req *subtaskRequest) validate() error {
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return errors.New("title required")
	}
	if len(req.Title) > 500 {
		return errors.New("title too long")
	}
	return nil
}

// Index 列出某个 todo 下的子任务。GET /api/todos/{todo_id}/subtasks
func (h *SubtasksHandler) Index(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	todoID, err := parseInt64(r.PathValue("todo_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid todo_id")
		return
	}
	items, err := h.Subtasks.ListByTodo(r.Context(), uid, todoID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*models.Subtask{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Create POST /api/todos/{todo_id}/subtasks
func (h *SubtasksHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	todoID, err := parseInt64(r.PathValue("todo_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid todo_id")
		return
	}
	var req subtaskRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	out, err := h.Subtasks.Create(r.Context(), uid, todoID, store.SubtaskInput{
		Title:     req.Title,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// Update PUT /api/subtasks/{id}
func (h *SubtasksHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req subtaskRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	out, err := h.Subtasks.Update(r.Context(), uid, id, req.Title, req.SortOrder)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Delete DELETE /api/subtasks/{id}
func (h *SubtasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Subtasks.Delete(r.Context(), uid, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Complete POST /api/subtasks/{id}/complete
func (h *SubtasksHandler) Complete(w http.ResponseWriter, r *http.Request) {
	h.setCompleted(w, r, true)
}

// Uncomplete POST /api/subtasks/{id}/uncomplete
func (h *SubtasksHandler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	h.setCompleted(w, r, false)
}

func (h *SubtasksHandler) setCompleted(w http.ResponseWriter, r *http.Request, completed bool) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	out, err := h.Subtasks.SetCompleted(r.Context(), uid, id, completed)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

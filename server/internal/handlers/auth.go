package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/youruser/todoalarm/internal/auth"
	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/store"
)

type AuthHandler struct {
	Issuer        *auth.Issuer
	Users         *store.UserStore
	RefreshTokens *store.RefreshTokenStore
}

func NewAuthHandler(issuer *auth.Issuer, users *store.UserStore, refresh *store.RefreshTokenStore) *AuthHandler {
	return &AuthHandler{Issuer: issuer, Users: users, RefreshTokens: refresh}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
	DeviceID    string `json:"device_id"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
	AllDevices   bool   `json:"all_devices"`
}

// authResponse 登录/注册/刷新成功后的响应。
type authResponse struct {
	AccessToken           string `json:"access_token"`
	AccessTokenExpiresAt  string `json:"access_token_expires_at"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresAt string `json:"refresh_token_expires_at"`
	User                  any    `json:"user"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateRegister(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	pwHash, err := h.Issuer.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	user, err := h.Users.Create(r.Context(), req.Email, pwHash, req.DisplayName, req.Timezone)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "email_taken", "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.issueAndWrite(w, r, user.ID, req.DeviceID, user, http.StatusCreated)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email and password required")
		return
	}
	user, hash, err := h.Users.GetByEmailWithHash(r.Context(), req.Email)
	if err != nil {
		// 模糊错误信息防爆破
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if !auth.CheckPassword(hash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	h.issueAndWrite(w, r, user.ID, req.DeviceID, user, http.StatusOK)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "refresh_token required")
		return
	}
	hash := auth.HashRefreshToken(req.RefreshToken)
	uid, err := h.RefreshTokens.LookupActive(r.Context(), hash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token invalid or expired")
		return
	}
	user, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "user no longer exists")
		return
	}
	// 刷新 = 旋转:撤销旧 refresh,签发新对。
	if err := h.RefreshTokens.Revoke(r.Context(), hash); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.issueAndWrite(w, r, user.ID, "", user, http.StatusOK)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := readJSON(r, &req); err != nil {
		// logout 不强制 body
		req = logoutRequest{}
	}
	uid := middleware.UserIDFrom(r.Context())
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	if req.AllDevices {
		if err := h.RefreshTokens.RevokeAllForUser(r.Context(), uid); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	} else if req.RefreshToken != "" {
		_ = h.RefreshTokens.Revoke(r.Context(), auth.HashRefreshToken(req.RefreshToken))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	user, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type updateMeRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Timezone    *string `json:"timezone,omitempty"`
}

// UpdateMe PATCH /api/auth/me
//
// 仅允许用户修改自己的 display_name / timezone。邮箱/密码改动走单独流程。
// 时区使用 IANA 名称，例如 Asia/Shanghai。空字符串视为未提供。
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var req updateMeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var displayName, timezone *string
	if req.DisplayName != nil {
		v := strings.TrimSpace(*req.DisplayName)
		if utf8.RuneCountInString(v) > 64 {
			writeError(w, http.StatusBadRequest, "bad_request", "display_name too long")
			return
		}
		displayName = &v
	}
	if req.Timezone != nil {
		v := strings.TrimSpace(*req.Timezone)
		if v != "" {
			if _, err := time.LoadLocation(v); err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid timezone")
				return
			}
		}
		timezone = &v
	}
	user, err := h.Users.Update(r.Context(), uid, displayName, timezone)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) issueAndWrite(w http.ResponseWriter, r *http.Request, userID int64, deviceID string, user any, status int) {
	now := time.Now().UTC()
	access, accessExp, err := h.Issuer.IssueAccessToken(userID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	refresh, refreshHash, refreshExp, err := h.Issuer.IssueRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := h.RefreshTokens.Create(r.Context(), userID, refreshHash, deviceID, refreshExp); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, status, authResponse{
		AccessToken:           access,
		AccessTokenExpiresAt:  accessExp.UTC().Format(time.RFC3339),
		RefreshToken:          refresh,
		RefreshTokenExpiresAt: refreshExp.UTC().Format(time.RFC3339),
		User:                  user,
	})
}

func validateRegister(req *registerRequest) error {
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		return errors.New("email required")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return errors.New("invalid email")
	}
	if utf8.RuneCountInString(req.Password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if utf8.RuneCountInString(req.Password) > 128 {
		return errors.New("password too long")
	}
	if utf8.RuneCountInString(req.DisplayName) > 80 {
		return errors.New("display_name too long")
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}
	return nil
}

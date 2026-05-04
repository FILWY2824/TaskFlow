package oauth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserInfoPicksQiShuEmailNameAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization = %q, want Bearer access-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"sub": "qishu-user-1",
			"email_name": "alice@example.com",
			"nickname": "Alice"
		}`))
	}))
	defer srv.Close()

	p := NewProvider("qishu", "", "", srv.URL, "", "", "", "", nil, "email", "name", "sub")
	info, err := p.UserInfo(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if info.Subject != "qishu-user-1" {
		t.Fatalf("subject = %q, want qishu-user-1", info.Subject)
	}
	if info.Email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", info.Email)
	}
	if info.DisplayName != "Alice" {
		t.Fatalf("display_name = %q, want Alice", info.DisplayName)
	}
}

func TestResolveUserInfoMergesOIDCIDTokenClaims(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"sub": "qishu-user-1",
			"name": "fallback name"
		}`))
	}))
	defer srv.Close()

	idToken := unsignedJWT(`{
		"sub": "qishu-user-1",
		"email": "real@example.com",
		"email_verified": true,
		"name": "真实姓名",
		"preferred_username": "real-user"
	}`)
	p := NewProvider("qishu", "", "", srv.URL, "taskflow-client", "", "", "", nil, "email", "name", "sub")

	info, err := p.ResolveUserInfo(context.Background(), &TokenResponse{
		AccessToken: "access-token",
		IDToken:     idToken,
	})
	if err != nil {
		t.Fatalf("resolve userinfo: %v", err)
	}
	if info.Subject != "qishu-user-1" {
		t.Fatalf("subject = %q, want qishu-user-1", info.Subject)
	}
	if info.Email != "real@example.com" {
		t.Fatalf("email = %q, want real@example.com", info.Email)
	}
	if info.DisplayName != "真实姓名" {
		t.Fatalf("display_name = %q, want 真实姓名", info.DisplayName)
	}
}

func unsignedJWT(payload string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return strings.Join([]string{header, body, ""}, ".")
}

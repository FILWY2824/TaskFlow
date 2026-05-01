package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseStartCommand(t *testing.T) {
	cases := []struct {
		in       string
		wantPL   string
		wantStrt bool
	}{
		{"/start bind_abc", "bind_abc", true},
		{"/start  bind_abc  ", "bind_abc", true},
		{"/start", "", true},
		{"/start@TaskFlowBot bind_xyz", "bind_xyz", true},
		{"/help", "", false},
		{"hello world", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		gotPL, gotStrt := ParseStartCommand(c.in)
		if gotPL != c.wantPL || gotStrt != c.wantStrt {
			t.Errorf("ParseStartCommand(%q) = (%q,%v), want (%q,%v)", c.in, gotPL, gotStrt, c.wantPL, c.wantStrt)
		}
	}
}

func TestExtractBindToken(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"bind_abc":                         {"abc", true},
		"bind_":                            {"", false},
		"hello":                            {"", false},
		"":                                 {"", false},
		"bind_long_token_with_underscores": {"long_token_with_underscores", true},
	}
	for in, want := range cases {
		got, ok := ExtractBindToken(in)
		if got != want.want || ok != want.ok {
			t.Errorf("ExtractBindToken(%q) = (%q,%v), want (%q,%v)", in, got, ok, want.want, want.ok)
		}
	}
}

func TestClient_SendMessage_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 路径形如 /botTOKEN/sendMessage
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content-type, got %q", r.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode req: %v", err)
		}
		if body["chat_id"] != "12345" {
			t.Errorf("chat_id = %v, want 12345", body["chat_id"])
		}
		if body["text"] != "hello" {
			t.Errorf("text = %v, want hello", body["text"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":42,"chat":{"id":12345,"type":"private"},"date":1700000000,"text":"hello","from":{"id":1,"is_bot":true}}}`)
	}))
	defer ts.Close()

	c := NewClient("TEST_TOKEN", ts.URL)
	if !c.Enabled() {
		t.Fatal("expected enabled")
	}
	msg, err := c.SendMessage(context.Background(), "12345", "hello", SendMessageOptions{})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msg.MessageID != 42 || msg.Chat.ID != 12345 {
		t.Errorf("unexpected msg: %+v", msg)
	}
}

func TestClient_SendMessage_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
	}))
	defer ts.Close()

	c := NewClient("TEST", ts.URL)
	_, err := c.SendMessage(context.Background(), "12345", "hi", SendMessageOptions{})
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("expected chat-not-found error, got %v", err)
	}
}

func TestClient_Disabled(t *testing.T) {
	c := NewClient("", "")
	if c.Enabled() {
		t.Fatal("expected disabled when token empty")
	}
	_, err := c.SendMessage(context.Background(), "1", "x", SendMessageOptions{})
	if err == nil {
		t.Error("expected error from disabled client")
	}
}

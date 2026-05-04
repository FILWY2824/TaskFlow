package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	taskflowdb "github.com/youruser/taskflow/internal/db"
)

func newUserStoreForTest(t *testing.T) (*UserStore, func()) {
	t.Helper()
	database, err := taskflowdb.Open(filepath.Join(t.TempDir(), "taskflow.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := taskflowdb.Migrate(ctx, database); err != nil {
		_ = database.Close()
		t.Fatalf("migrate db: %v", err)
	}
	return NewUserStore(database), func() {
		_ = database.Close()
	}
}

func TestUserStoreDefaultsTimezoneToShanghai(t *testing.T) {
	users, cleanup := newUserStoreForTest(t)
	defer cleanup()

	u, err := users.Create(context.Background(), "alice@example.com", "hash", "Alice", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q, want Asia/Shanghai", u.Timezone)
	}
}

func TestUserStoreUpdateEmptyTimezoneResetsToShanghai(t *testing.T) {
	users, cleanup := newUserStoreForTest(t)
	defer cleanup()

	u, err := users.Create(context.Background(), "bob@example.com", "hash", "Bob", "America/New_York")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	empty := ""
	u, err = users.Update(context.Background(), u.ID, nil, &empty)
	if err != nil {
		t.Fatalf("update user: %v", err)
	}
	if u.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q, want Asia/Shanghai", u.Timezone)
	}
}

func TestUserStoreUpsertOAuthUsesProvidedEmailAndShanghaiTimezone(t *testing.T) {
	users, cleanup := newUserStoreForTest(t)
	defer cleanup()

	u, err := users.UpsertOAuth(context.Background(), "qishu", "subject-123", "Alice@Example.COM", "Alice")
	if err != nil {
		t.Fatalf("upsert oauth: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", u.Email)
	}
	if u.DisplayName != "Alice" {
		t.Fatalf("display_name = %q, want Alice", u.DisplayName)
	}
	if u.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q, want Asia/Shanghai", u.Timezone)
	}
}

func TestUserStoreUpsertOAuthFallsBackToReadableProviderEmail(t *testing.T) {
	users, cleanup := newUserStoreForTest(t)
	defer cleanup()

	u, err := users.UpsertOAuth(context.Background(), "teamcy.eu.cc", "f03a863f-1148-4b65-8ae0-7ead9ff8c37f", "", "admin")
	if err != nil {
		t.Fatalf("upsert oauth: %v", err)
	}
	if u.Email != "admin@teamcy.eu.cc" {
		t.Fatalf("email = %q, want admin@teamcy.eu.cc", u.Email)
	}
}

func TestUserStoreUpsertOAuthReplacesSubjectFallbackWithReadableEmail(t *testing.T) {
	users, cleanup := newUserStoreForTest(t)
	defer cleanup()

	u, err := users.UpsertOAuth(context.Background(), "teamcy.eu.cc", "f03a863f-1148-4b65-8ae0-7ead9ff8c37f", "", "")
	if err != nil {
		t.Fatalf("upsert oauth: %v", err)
	}
	if u.Email != "f03a863f-1148-4b65-8ae0-7ead9ff8c37f@teamcy.eu.cc" {
		t.Fatalf("initial email = %q, want subject fallback", u.Email)
	}

	u, err = users.UpsertOAuth(context.Background(), "teamcy.eu.cc", "f03a863f-1148-4b65-8ae0-7ead9ff8c37f", "", "admin")
	if err != nil {
		t.Fatalf("upsert oauth again: %v", err)
	}
	if u.Email != "admin@teamcy.eu.cc" {
		t.Fatalf("email = %q, want admin@teamcy.eu.cc", u.Email)
	}
}

func TestUserStoreUpsertOAuthDoesNotOverwriteEditedDisplayName(t *testing.T) {
	users, cleanup := newUserStoreForTest(t)
	defer cleanup()

	u, err := users.UpsertOAuth(context.Background(), "qishu", "subject-456", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("upsert oauth: %v", err)
	}
	customName := "本地显示名"
	if _, err := users.Update(context.Background(), u.ID, &customName, nil); err != nil {
		t.Fatalf("update display name: %v", err)
	}

	u, err = users.UpsertOAuth(context.Background(), "qishu", "subject-456", "alice2@example.com", "QiShu Name")
	if err != nil {
		t.Fatalf("upsert oauth again: %v", err)
	}
	if u.Email != "alice2@example.com" {
		t.Fatalf("email = %q, want alice2@example.com", u.Email)
	}
	if u.DisplayName != customName {
		t.Fatalf("display_name = %q, want %q", u.DisplayName, customName)
	}
}

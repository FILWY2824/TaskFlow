package rrule

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestComputeNextFire_OneShot_Future(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trigger := now.Add(time.Hour)
	got, err := ComputeNextFire(&trigger, "", nil, "", now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil next")
	}
	if !got.Equal(trigger.UTC()) {
		t.Fatalf("want %v got %v", trigger, *got)
	}
}

func TestComputeNextFire_OneShot_Past(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trigger := now.Add(-time.Hour)
	got, err := ComputeNextFire(&trigger, "", nil, "", now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for past trigger, got %v", *got)
	}
}

func TestComputeNextFire_Empty(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := ComputeNextFire(nil, "", nil, "UTC", now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil")
	}
}

// 体检场景:每 6 个月一次,起点 2026-01-15 09:00 上海时间。
// 2026-02-01 之后的下一次应当是 2026-07-15 09:00 上海。
func TestComputeNextFire_Monthly6_HealthCheck(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	dtstart := time.Date(2026, 1, 15, 9, 0, 0, 0, loc)
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)

	got, err := ComputeNextFire(nil, "FREQ=MONTHLY;INTERVAL=6", &dtstart, "Asia/Shanghai", now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected next fire")
	}
	want := time.Date(2026, 7, 15, 9, 0, 0, 0, loc).UTC()
	if !got.Equal(want) {
		t.Fatalf("want %v got %v", want, *got)
	}
}

// 跨夏令时:纽约 2026-03-08 是 DST 开始日,凌晨 2:00 跳到 3:00。
// 但每天 09:00 提醒应当落在本地的 09:00,UTC 偏移会从 -5 变 -4。
func TestComputeNextFire_DailyAcrossDST(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	dtstart := time.Date(2026, 3, 7, 9, 0, 0, 0, loc) // 在 DST 之前
	// 我们在 3-8 9:30 (DST 后)询问下次:应该是 3-9 09:00 NY local。
	now := time.Date(2026, 3, 8, 9, 30, 0, 0, loc)

	got, err := ComputeNextFire(nil, "FREQ=DAILY", &dtstart, "America/New_York", now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected next fire")
	}
	want := time.Date(2026, 3, 9, 9, 0, 0, 0, loc).UTC()
	if !got.Equal(want) {
		t.Fatalf("want %v got %v", want, *got)
	}
}

func TestComputeNextFire_InvalidRRule(t *testing.T) {
	dtstart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := ComputeNextFire(nil, "FREQ=BOGUS", &dtstart, "UTC", now)
	if err == nil {
		t.Fatal("expected error for invalid rrule")
	}
}

func TestValidateRRule_OK(t *testing.T) {
	dtstart := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	if err := ValidateRRule("FREQ=WEEKLY;BYDAY=MO,WE,FR", "UTC", &dtstart); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateRRule_RequiresDtstart(t *testing.T) {
	if err := ValidateRRule("FREQ=DAILY", "UTC", nil); err == nil {
		t.Fatal("expected error when dtstart missing")
	}
}

func TestValidateRRule_EmptyOK(t *testing.T) {
	if err := ValidateRRule("", "", nil); err != nil {
		t.Fatalf("empty rrule should be ok: %v", err)
	}
}

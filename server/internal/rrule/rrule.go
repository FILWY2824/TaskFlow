package rrule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	rr "github.com/teambition/rrule-go"
)

// ComputeNextFire 计算下一个触发时刻(>= notBefore,严格 >)。
//
// 三种模式:
//  1. 单次提醒: triggerAt 非 nil, rrule 为空。如果 triggerAt > notBefore,返回 triggerAt;否则返回 nil(已过期)。
//  2. 周期提醒: rrule 非空, dtstart 非 nil。基于 RFC 5545 RRULE 计算下一次。
//  3. 都为空: 返回 nil(无下一次)。
//
// tzName 是 IANA 时区名(如 "Asia/Shanghai")。所有计算在该时区,但返回 UTC 时刻。
func ComputeNextFire(triggerAt *time.Time, rruleStr string, dtstart *time.Time, tzName string, notBefore time.Time) (*time.Time, error) {
	rruleStr = strings.TrimSpace(rruleStr)

	// 单次
	if rruleStr == "" {
		if triggerAt == nil {
			return nil, nil
		}
		if triggerAt.After(notBefore) {
			t := triggerAt.UTC()
			return &t, nil
		}
		return nil, nil
	}

	// 周期
	if dtstart == nil {
		// 没有 dtstart 视为非法
		return nil, errors.New("rrule requires dtstart")
	}
	loc, err := loadLocation(tzName)
	if err != nil {
		return nil, err
	}
	dts := dtstart.In(loc)

	// rrule-go 接受 "RRULE:..." 或 "FREQ=...";我们统一加前缀。
	rruleClean := rruleStr
	if !strings.HasPrefix(strings.ToUpper(rruleClean), "RRULE:") {
		rruleClean = "RRULE:" + rruleClean
	}
	opts, err := rr.StrToROptionInLocation(rruleClean, loc)
	if err != nil {
		return nil, fmt.Errorf("parse rrule: %w", err)
	}
	opts.Dtstart = dts

	rule, err := rr.NewRRule(*opts)
	if err != nil {
		return nil, fmt.Errorf("build rrule: %w", err)
	}

	// after(notBefore, false) 返回 > notBefore 的下一次。
	next := rule.After(notBefore.In(loc), false)
	if next.IsZero() {
		return nil, nil
	}
	out := next.UTC()
	return &out, nil
}

// ValidateRRule 在保存之前简单校验 RRULE 字符串。
func ValidateRRule(rruleStr, tzName string, dtstart *time.Time) error {
	rruleStr = strings.TrimSpace(rruleStr)
	if rruleStr == "" {
		return nil
	}
	if dtstart == nil {
		return errors.New("rrule requires dtstart")
	}
	loc, err := loadLocation(tzName)
	if err != nil {
		return err
	}
	r := rruleStr
	if !strings.HasPrefix(strings.ToUpper(r), "RRULE:") {
		r = "RRULE:" + r
	}
	opts, err := rr.StrToROptionInLocation(r, loc)
	if err != nil {
		return fmt.Errorf("invalid rrule: %w", err)
	}
	opts.Dtstart = dtstart.In(loc)
	if _, err := rr.NewRRule(*opts); err != nil {
		return fmt.Errorf("invalid rrule: %w", err)
	}
	return nil
}

func loadLocation(tzName string) (*time.Location, error) {
	if strings.TrimSpace(tzName) == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", tzName, err)
	}
	return loc, nil
}

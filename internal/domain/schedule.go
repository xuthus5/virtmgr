package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	ScheduleModeInterval = "interval"
	ScheduleModeDaily    = "daily"
)

func NormalizeScheduleMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ScheduleModeInterval:
		return ScheduleModeInterval, nil
	case ScheduleModeDaily:
		return ScheduleModeDaily, nil
	default:
		return "", fmt.Errorf("%w: unsupported schedule mode", ErrInvalidInput)
	}
}

func NormalizeDailyTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "02:00", nil
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return "", fmt.Errorf("%w: daily time must use HH:MM", ErrInvalidInput)
	}
	return parsed.Format("15:04"), nil
}

// NextScheduleTime uses the server's local timezone. DST transitions are
// intentionally not given special handling in this single-host implementation.
func NextScheduleTime(now time.Time, mode string, interval time.Duration, dailyTime string) time.Time {
	mode, err := NormalizeScheduleMode(mode)
	if err != nil || mode == ScheduleModeInterval {
		return now.Add(interval)
	}
	dailyTime, err = NormalizeDailyTime(dailyTime)
	if err != nil {
		return now.Add(interval)
	}
	parsed, _ := time.Parse("15:04", dailyTime)
	local := now.In(time.Local)
	next := time.Date(local.Year(), local.Month(), local.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.Local)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next.UTC()
}

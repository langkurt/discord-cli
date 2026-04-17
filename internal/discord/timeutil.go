package discord

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseSince parses a --since value into a UTC time.Time.
// Accepts relative values like "90d", "30d", "7d", "6m", "1y"
// and absolute dates like "2026-01-01".
func ParseSince(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty since value")
	}

	// Try absolute date (Go's reference time is 2006-01-02)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}

	// Relative: digits followed by d/m/y
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil || n <= 0 {
		return time.Time{}, fmt.Errorf("invalid --since %q: use e.g. 30d, 6m, 1y or 2026-01-01", s)
	}

	now := time.Now().UTC()
	switch unit {
	case 'd':
		return now.AddDate(0, 0, -n), nil
	case 'm':
		return now.AddDate(0, -n, 0), nil
	case 'y':
		return now.AddDate(-n, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("invalid --since %q: unit must be d, m, or y", s)
	}
}

// TimeToSnowflake converts a time.Time to a Discord snowflake ID string.
// Snowflakes are 64-bit integers where the top 42 bits encode milliseconds
// since the Discord epoch (2015-01-01). The result can be used as a message
// ID boundary in ChannelMessages API calls.
func TimeToSnowflake(t time.Time) string {
	const discordEpoch int64 = 1420070400000 // ms since Unix epoch
	ms := t.UnixMilli()
	snowflake := (ms - discordEpoch) << 22
	if snowflake < 0 {
		snowflake = 0
	}
	return strconv.FormatInt(snowflake, 10)
}

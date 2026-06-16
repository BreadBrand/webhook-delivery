package db

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	// SQLite datetime('now') format: space-separated, no timezone suffix
	t.Run("SQLite format", func(t *testing.T) {
		got := parseTime("2024-03-15 12:30:45")
		want := time.Date(2024, 3, 15, 12, 30, 45, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("parseTime(SQLite) = %v, want %v", got, want)
		}
		if got.Location() != time.UTC {
			t.Errorf("parseTime(SQLite) location = %v, want UTC", got.Location())
		}
	})

	// RFC3339 / ISO 8601 with Z suffix
	t.Run("RFC3339 format", func(t *testing.T) {
		got := parseTime("2024-03-15T12:30:45Z")
		want := time.Date(2024, 3, 15, 12, 30, 45, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("parseTime(RFC3339) = %v, want %v", got, want)
		}
	})

	// RFC3339 with timezone offset
	t.Run("RFC3339 with offset", func(t *testing.T) {
		got := parseTime("2024-03-15T12:30:45+05:00")
		want := time.Date(2024, 3, 15, 7, 30, 45, 0, time.UTC) // normalized to UTC
		if !got.Equal(want) {
			t.Errorf("parseTime(RFC3339 offset) = %v, want %v", got, want)
		}
	})

	// Garbage input should return zero time
	t.Run("garbage input", func(t *testing.T) {
		got := parseTime("not-a-date")
		if !got.IsZero() {
			t.Errorf("parseTime(garbage) = %v, want zero time", got)
		}
	})

	// Empty string should return zero time
	t.Run("empty string", func(t *testing.T) {
		got := parseTime("")
		if !got.IsZero() {
			t.Errorf("parseTime(\"\") = %v, want zero time", got)
		}
	})
}

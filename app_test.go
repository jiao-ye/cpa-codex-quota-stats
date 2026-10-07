package main

import (
	"database/sql"

	"net/http"

	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRecordUsageStoresQuotaObservationTime(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "observed-at.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	a := &app{cfg: defaultConfig(), store: s}
	requestedAt := time.Unix(100, 0)
	record := usageRecord{
		Provider:    "openai",
		Model:       "gpt",
		AuthID:      "observed-account",
		RequestedAt: requestedAt,
		TTFT:        int64(90 * time.Second),
		Detail:      usageDetail{TotalTokens: 100},
		ResponseHeaders: http.Header{
			"X-Codex-Primary-Used-Percent":   {"40"},
			"X-Codex-Primary-Reset-At":       {"700"},
			"X-Codex-Primary-Window-Minutes": {"10"},
			"X-Codex-Plan-Type":              {"pro"},
		},
	}
	if err = a.recordUsage(record); err != nil {
		t.Fatal(err)
	}
	var requested, observed int64
	if err = s.db.QueryRow(`SELECT requested_at,observed_at FROM usage_events LIMIT 1`).Scan(&requested, &observed); err != nil {
		t.Fatal(err)
	}
	if requested != 100 || observed != 190 {
		t.Fatalf("requested=%d observed=%d", requested, observed)
	}
}

func TestRecordUsageStoresWeeklySecondaryQuotaOnlyForFiveHourPrimary(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "secondary-quota.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	a := &app{cfg: defaultConfig(), store: s}

	record := func(account string, primaryWindow int64) {
		t.Helper()
		if err := a.recordUsage(usageRecord{
			Provider:    "openai",
			Model:       "gpt",
			AuthID:      account,
			RequestedAt: time.Unix(1_000, 0),
			Detail:      usageDetail{TotalTokens: 100},
			ResponseHeaders: http.Header{
				"X-Codex-Primary-Used-Percent":     {"12"},
				"X-Codex-Primary-Reset-At":         {"19001"},
				"X-Codex-Primary-Window-Minutes":   {stringInt(primaryWindow)},
				"X-Codex-Secondary-Used-Percent":   {"34"},
				"X-Codex-Secondary-Reset-At":       {"605801"},
				"X-Codex-Secondary-Window-Minutes": {"10080"},
				"X-Codex-Plan-Type":                {"plus"},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	record("five-hour-account", 300)
	record("weekly-only-account", 10080)

	var used sql.NullFloat64
	var resetAt, windowMinutes int64
	if err = s.db.QueryRow(`SELECT secondary_used_percent,secondary_reset_at,secondary_window_minutes FROM usage_events WHERE account=?`, "five-hour-account").Scan(&used, &resetAt, &windowMinutes); err != nil {
		t.Fatal(err)
	}
	if !used.Valid || used.Float64 != 34 || resetAt != 605_820 || windowMinutes != 10080 {
		t.Fatalf("stored secondary quota = used:%#v reset:%d window:%d", used, resetAt, windowMinutes)
	}
	if err = s.db.QueryRow(`SELECT secondary_used_percent,secondary_reset_at,secondary_window_minutes FROM usage_events WHERE account=?`, "weekly-only-account").Scan(&used, &resetAt, &windowMinutes); err != nil {
		t.Fatal(err)
	}
	if used.Valid || resetAt != 0 || windowMinutes != 0 {
		t.Fatalf("weekly-only account unexpectedly enabled secondary quota = used:%#v reset:%d window:%d", used, resetAt, windowMinutes)
	}
}

func stringInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func auditStore(t *testing.T) *store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "audit.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.close() })
	return s
}

func auditInsert(t *testing.T, s *store, e event) {
	t.Helper()
	if err := s.insertEvent(context.Background(), e, time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestLocalAuditMonthlyExcludesConfirmedQuotaAnomaly(t *testing.T) {
	s := auditStore(t)
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, shanghaiLocation()).Unix()
	reset := base + 10080*60
	for i, row := range []struct {
		used  float64
		reset int64
	}{
		{49, reset}, {50, reset},
		{70, reset + 600}, {71, reset + 600},
		{50, reset}, {51, reset},
	} {
		used := row.used
		auditInsert(t, s, event{
			Account: "anomaly", RequestedAt: base + int64(i+1)*100,
			ObservedAt: base + int64(i+1)*100, Model: "gpt-5.6-sol",
			TotalTokens: 100, CostUSD: 1, UsedPercent: &used,
			ResetAt: row.reset, WindowMinutes: 10080, PlanType: "pro",
		})
	}
	cycles, err := s.cycles(context.Background(), "anomaly", 10)
	if err != nil || len(cycles) != 1 || cycles[0].PeakPercent != 51 {
		t.Fatalf("fixture must recover to 51%%: cycles=%+v err=%v", cycles, err)
	}
	anomalies, err := s.quotaRegimeAnomalies(context.Background(), cycles[0].ID)
	if err != nil || len(anomalies) != 1 {
		t.Fatalf("fixture must have one confirmed anomaly: %+v err=%v", anomalies, err)
	}
	summary, err := s.monthly(context.Background(), "anomaly", "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if summary.ConsumedQuotaPercent != 51 {
		t.Fatalf("monthly consumption=%g%%, want 51%%; active cycle peak=%g%%",
			summary.ConsumedQuotaPercent, cycles[0].PeakPercent)
	}
	accounting, err := s.accounting(context.Background(), "anomaly", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounting.LifetimeQuota) != 1 || accounting.LifetimeQuota[0].Percent != 2 {
		t.Fatalf("permanent observed growth must exclude anomalous 71%% and initial unobserved 49%%: %+v", accounting.LifetimeQuota)
	}
}

func TestLocalAuditLateOldResponsePreservesNewResetCycle(t *testing.T) {
	s := auditStore(t)
	oldReset := time.Date(2026, 9, 10, 12, 0, 0, 0, shanghaiLocation()).Unix()
	newReset := oldReset + 10080*60
	insert := func(requested, observed int64, used float64, reset int64) {
		auditInsert(t, s, event{
			Account: "late", RequestedAt: requested, ObservedAt: observed,
			Model: "gpt-5.6-sol", TotalTokens: 100,
			UsedPercent: &used, ResetAt: reset, WindowMinutes: 10080, PlanType: "pro",
		})
	}
	insert(oldReset-100, oldReset-90, 40, oldReset)
	insert(oldReset+10, oldReset+11, 0, newReset)
	insert(oldReset+20, oldReset+21, 1, newReset)
	before, err := s.cycles(context.Background(), "late", 10)
	if err != nil || len(before) != 2 || before[0].ResetAt != newReset {
		t.Fatalf("fixture must confirm new cycle: %+v err=%v", before, err)
	}
	// A slow request can report headers observed before reset but finish later.
	insert(oldReset-50, oldReset-40, 41, oldReset)
	after, err := s.cycles(context.Background(), "late", 10)
	if err != nil || len(after) != 2 {
		t.Fatalf("cycles=%+v err=%v", after, err)
	}
	if after[0].ResetAt != newReset || after[0].ActualTokens != 200 || after[1].ActualTokens != 200 {
		t.Fatalf("late response corrupted cycle state or attribution: new=%+v old=%+v",
			after[0], after[1])
	}
}

func TestLocalAuditSecondaryWeeklyResetAfterIdleGap(t *testing.T) {
	s := auditStore(t)
	oldReset := time.Date(2026, 9, 10, 12, 0, 0, 0, shanghaiLocation()).Unix()
	newStart := oldReset + 900
	newReset := newStart + 10080*60
	insert := func(at int64, primary, secondary float64, primaryReset, secondaryReset int64) {
		auditInsert(t, s, event{
			Account: "weekly", RequestedAt: at, ObservedAt: at + 1,
			Model: "gpt-5.6-sol", TotalTokens: 100,
			UsedPercent: &primary, ResetAt: primaryReset, WindowMinutes: 300,
			SecondaryUsedPercent: &secondary, SecondaryResetAt: secondaryReset,
			SecondaryWindowMinutes: 10080, PlanType: "plus",
		})
	}
	insert(oldReset-100, 80, 98, oldReset, oldReset)
	insert(newStart+10, 0, 0, newStart+300*60, newReset)
	insert(newStart+20, 1, 1, newStart+300*60, newReset)
	cycles, err := s.quotaScopeCycles(context.Background(), "weekly", weeklyQuotaScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(cycles) != 2 || cycles[1].PeakPercent != 98 || cycles[1].EndedAt != oldReset {
		t.Fatalf("weekly idle reset must retain old and new cycles; got %+v", cycles)
	}
}

func TestLocalAuditCleanupPreservesLifetimeTotal(t *testing.T) {
	s := auditStore(t)
	oldAt := time.Now().Add(-366 * 24 * time.Hour).Unix()
	auditInsert(t, s, event{
		Account: "lifetime", RequestedAt: oldAt, TotalTokens: 1234,
		Model: "gpt-5.6-sol",
	})
	if err := s.cleanup(context.Background(), 365); err != nil {
		t.Fatal(err)
	}
	total, err := s.lifetimeAccounting(context.Background(), "lifetime")
	if err != nil {
		t.Fatal(err)
	}
	if total.TotalTokens != 1234 {
		t.Fatalf("retained lifetime total=%d, want 1234", total.TotalTokens)
	}
}

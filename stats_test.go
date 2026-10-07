package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

func TestStatsOnlyRoutesAndDashboard(t *testing.T) {
	s := auditStore(t)
	a := &app{cfg: defaultConfig(), store: s}
	for _, path := range []string{"/weights", "/weights/backtest", "/calibration", "/calibration/start", "/summary", "/series", "/monthly", "/pricing-settings", "/forecast", "/capacity"} {
		for _, method := range []string{"GET", "POST"} {
			resp := a.handleManagement(managementRequest{Method: method, Path: "/" + pluginID + path})
			if resp.StatusCode != 404 {
				t.Fatalf("%s %s remains exposed: %d", method, path, resp.StatusCode)
			}
		}
	}
	for _, word := range []string{"estimated_", "forecast", "capacity", "calibration", "learned", "预测", "校准", "耗尽"} {
		if bytes.Contains(bytes.ToLower(dashboardHTML), []byte(word)) {
			t.Fatalf("dashboard includes removed feature %q", word)
		}
	}
	reg, err := json.Marshal(managementRegistration())
	if err != nil || bytes.Contains(reg, []byte("weights")) || bytes.Contains(reg, []byte("预测")) {
		t.Fatalf("registration=%s %v", reg, err)
	}
	for _, path := range []string{"/evil/accounting", "/not-a-dashboard", "/" + pluginID + "/overview/extra"} {
		if got := a.handleManagement(managementRequest{Method: "GET", Path: path}); got.StatusCode != 404 {
			t.Fatalf("suffix route matched: %s", path)
		}
	}
	if response := a.handleManagement(managementRequest{Method: "GET", Path: "/" + pluginID + "/dashboard"}); response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	for _, path := range []string{"/" + pluginID + "/overview", "/v0/management/" + pluginID + "/overview"} {
		if response := a.handleManagement(managementRequest{Method: "GET", Path: path}); response.StatusCode != 200 {
			t.Fatalf("mounted route %s: %d", path, response.StatusCode)
		}
	}
	if response := a.handleManagement(managementRequest{Method: "GET", Path: "/v0/resource/plugins/" + pluginID + "/dashboard"}); response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
}

func TestStatsBreakdownIncludesEveryScopeAndArchivedRecords(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	base := time.Now().Add(-400 * 24 * time.Hour).Unix()
	for _, e := range []event{
		{Account: "a", RequestedAt: base, Model: "standard-model", TotalTokens: 100},
		{Account: "a", RequestedAt: base + 1, Model: "spark-model", QuotaScope: sparkQuotaScope, TotalTokens: 200},
		{Account: "a", RequestedAt: base + 2, Model: "failed-model", Failed: true, TotalTokens: 3},
		{Account: "other", RequestedAt: base, Model: "other", TotalTokens: 999},
	} {
		auditInsert(t, s, e)
	}
	if err := s.cleanup(ctx, 365); err != nil {
		t.Fatal(err)
	}
	rows, err := s.actualUsageBreakdown(ctx, "a", base, base+3)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	var total int64
	for _, r := range rows {
		total += r.Totals.TotalTokens
	}
	if total != 303 {
		t.Fatalf("archived or Spark usage omitted: %d", total)
	}
	rows, err = s.actualUsageBreakdown(ctx, "a", base+1, base+2)
	if err != nil || len(rows) != 1 || rows[0].Totals.TotalTokens != 200 {
		t.Fatalf("half-open range=%+v %v", rows, err)
	}
}

func TestStatsRetainsExpiredObservationsWithoutCreatingFutureCycles(t *testing.T) {
	s := auditStore(t)
	base := time.Now().Add(-20 * 24 * time.Hour).Unix()
	used := 30.0
	auditInsert(t, s, event{Account: "expired", QuotaScope: sparkQuotaScope, RequestedAt: base, ObservedAt: base,
		TotalTokens: 42, UsedPercent: &used, ResetAt: base + 3600, WindowMinutes: 300})
	for _, now := range []int64{base + 7200, base + 50*86400} {
		series, err := s.latestQuotaScopeSeriesAt(context.Background(), "expired", sparkQuotaScope, 50, now)
		if err != nil || series.UsedPercent != 30 || series.ResetAt != base+3600 || series.ScheduleInferred {
			t.Fatalf("synthetic cycle: %+v %v", series, err)
		}
	}
	r, err := s.accounting(context.Background(), "expired", 50, 0)
	if err != nil || len(r.Resets) != 1 || len(r.Observations) != 1 || r.Observations[0].Used != 30 || r.Lifetime.TotalTokens != 42 {
		t.Fatalf("expired evidence=%+v %v", r, err)
	}
}

func TestStatsUpgradeLeavesLegacyLearningTablesUntouched(t *testing.T) {
	s := auditStore(t)
	for _, table := range []string{"weight_fits", "quota_segments", "online_cycle_scales", "calibration_sessions"} {
		if _, err := s.db.Exec(`CREATE TABLE ` + table + ` (legacy TEXT); INSERT INTO ` + table + ` VALUES('untouched')`); err != nil {
			t.Fatal(err)
		}
	}
	auditInsert(t, s, event{Account: "upgrade", RequestedAt: 100, TotalTokens: 123})
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: defaultConfig(), store: s}
	if err := a.recordUsage(usageRecord{AuthID: "upgrade", Detail: usageDetail{TotalTokens: 10}}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"weight_fits", "quota_segments", "online_cycle_scales", "calibration_sessions"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE legacy='untouched'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s modified: %d %v", table, n, err)
		}
	}
	total, err := s.lifetimeAccounting(context.Background(), "upgrade")
	if err != nil || total.TotalTokens != 133 {
		t.Fatalf("upgrade ledger=%+v %v", total, err)
	}
	// Responses contain only permanent statistical data.
	resp := a.handleManagement(managementRequest{Method: "GET", Path: "/" + pluginID + "/accounting", Query: url.Values{"account": {"upgrade"}}})
	if resp.StatusCode != 200 || bytes.Contains(resp.Body, []byte("estimated_")) || bytes.Contains(resp.Body, []byte("learned")) {
		t.Fatalf("response=%s", resp.Body)
	}
}

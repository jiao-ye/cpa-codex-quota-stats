package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountingPeriodsReconcileAfterCleanupAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-400 * 24 * time.Hour).Unix()
	for i := 0; i < 6; i++ {
		e := event{IngestID: fmt.Sprintf("event-%d", i), RequestedAt: base + int64(i)*100, ObservedAt: base + int64(i)*100,
			Account: "account-a", Model: "gpt-5.6-sol", InputTokens: 80, OutputTokens: 20, TotalTokens: 100}
		auditInsert(t, s, e)
		// Ambiguous commit retries must not increment either ledger.
		auditInsert(t, s, e)
	}
	auditInsert(t, s, event{Account: "account-b", RequestedAt: base, TotalTokens: 999})
	for _, at := range []int64{base, base + 200, base + 400} {
		if err = s.savePayment(ctx, subscriptionPayment{Account: "account-a", PaidAt: at, Amount: 20}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.cleanup(ctx, 365); err != nil {
		t.Fatal(err)
	}
	var raw int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE account='account-a'`).Scan(&raw); err != nil || raw != 0 {
		t.Fatalf("cleanup raw=%d err=%v", raw, err)
	}
	if err = s.close(); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	report, err := s.accounting(ctx, "account-a", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Lifetime.TotalTokens != 600 || report.Lifetime.Requests != 6 || len(report.Subscriptions) != 3 {
		t.Fatalf("report=%+v", report)
	}
	var sum int64
	for _, p := range report.Subscriptions {
		if p.Totals.TotalTokens != 200 {
			t.Fatalf("half-open boundary violated: %+v", p)
		}
		sum += p.Totals.TotalTokens
	}
	if sum != report.Lifetime.TotalTokens {
		t.Fatalf("periods=%d lifetime=%d", sum, report.Lifetime.TotalTokens)
	}
	// A retried old ingestion is still idempotent after detail expiry.
	auditInsert(t, s, event{IngestID: "event-0", RequestedAt: base, Account: "account-a", TotalTokens: 100})
	totals, _ := s.lifetimeAccounting(ctx, "account-a")
	if totals.TotalTokens != 600 {
		t.Fatalf("expired duplicate counted: %+v", totals)
	}
	accounts, err := s.accounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("headerless accounts=%v err=%v", accounts, err)
	}
}

func TestAccountingRateSnapshotsSeparateUnitsAndSurviveRepricing(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	if err := seedPrices(ctx, s); err != nil {
		t.Fatal(err)
	}
	e := event{IngestID: "priced", Account: "rates", RequestedAt: time.Now().Unix(), Model: "gpt-5.6-sol",
		InputTokens: 1_000_000, OutputTokens: 100_000, ReasoningTokens: 50_000, CacheReadTokens: 100_000, TotalTokens: 1_100_000, CostUSD: 123}
	auditInsert(t, s, e)
	got, err := s.lifetimeAccounting(ctx, "rates")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.USDReference-10.28) > 1e-9 || math.Abs(got.CreditReference-141) > 1e-9 || got.TotalTokens != 1_100_000 {
		t.Fatalf("units=%+v", got)
	}
	if _, err = s.db.Exec(`UPDATE usage_events SET cost_usd=999; UPDATE model_prices SET input=999`); err != nil {
		t.Fatal(err)
	}
	after, _ := s.lifetimeAccounting(ctx, "rates")
	if after != got {
		t.Fatalf("repricing rewrote immutable accounting: before=%+v after=%+v", got, after)
	}
	auditInsert(t, s, event{Account: "rates", RequestedAt: e.RequestedAt, Model: "unpublished", TotalTokens: 1})
	after, _ = s.lifetimeAccounting(ctx, "rates")
	if after.USDUnpriced != 1 || after.CreditUnpriced != 1 {
		t.Fatalf("missing rates hidden: %+v", after)
	}
}

func TestAccountingFailureRollsBackEveryLedger(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`CREATE TRIGGER inject_disk_failure BEFORE INSERT ON usage_ledger BEGIN SELECT RAISE(ABORT,'injected disk failure'); END`); err != nil {
		t.Fatal(err)
	}
	used := 10.0
	e := event{IngestID: "retry", Account: "rollback", RequestedAt: time.Now().Unix(), TotalTokens: 42,
		UsedPercent: &used, ResetAt: time.Now().Add(5 * time.Hour).Unix(), WindowMinutes: 300}
	if err := s.insertEvent(ctx, e, time.Minute); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, table := range []string{"usage_events", "usage_ledger", "lifetime_ledger", "quota_cycles", "quota_samples"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s partially committed: n=%d err=%v", table, n, err)
		}
	}
	if _, err := s.db.Exec(`DROP TRIGGER inject_disk_failure`); err != nil {
		t.Fatal(err)
	}
	auditInsert(t, s, e)
	auditInsert(t, s, e)
	total, _ := s.lifetimeAccounting(ctx, "rollback")
	if total.TotalTokens != 42 || total.Requests != 1 {
		t.Fatalf("retry=%+v", total)
	}
}

func TestAccountingRealSQLiteFullIsAtomic(t *testing.T) {
	s := auditStore(t)
	var pages int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA max_page_count=%d`, pages)); err != nil {
		t.Fatal(err)
	}
	e := event{Account: "full", RequestedAt: 100, Model: strings.Repeat("x", 1<<20), TotalTokens: 999}
	err := s.insertEvent(context.Background(), e, time.Minute)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "full") {
		t.Fatalf("expected real SQLITE_FULL: %v", err)
	}
	got, err := s.lifetimeAccounting(context.Background(), "full")
	if err != nil || got.Requests != 0 {
		t.Fatalf("failed write was accounted: %+v %v", got, err)
	}
	var count int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM usage_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial raw event: %d %v", count, err)
	}
}

func TestAccountingConcurrentIngestionReconciles(t *testing.T) {
	s := auditStore(t)
	var wg sync.WaitGroup
	errors := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := event{Account: "concurrent", IngestID: fmt.Sprintf("concurrent-%d", i), RequestedAt: 100 + int64(i), TotalTokens: 100}
			errors <- s.insertEvent(context.Background(), e, time.Minute)
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.lifetimeAccounting(context.Background(), "concurrent")
	if err != nil || got.TotalTokens != 4000 || got.Requests != 40 {
		t.Fatalf("concurrency=%+v %v", got, err)
	}
}

func TestAccountingOnlineBackupCanBeReopened(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	auditInsert(t, s, event{Account: "backup", RequestedAt: 100, TotalTokens: 500})
	if err := s.savePayment(ctx, subscriptionPayment{Account: "backup", PaidAt: 50, Amount: 20}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.sqlite")
	if _, err := s.db.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	restored, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.close()
	report, err := restored.accounting(ctx, "backup", 50, 0)
	if err != nil || report.Lifetime.TotalTokens != 500 || len(report.Subscriptions) != 1 || report.Subscriptions[0].Totals.TotalTokens != 500 {
		t.Fatalf("restored=%+v err=%v", report, err)
	}
}

func TestAccountingScopedHistorySurvivesDetailCleanup(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	base := time.Now().Add(-400 * 24 * time.Hour).Unix()
	oldReset := base + 100
	for _, scope := range []string{mainQuotaScope, sparkQuotaScope} {
		for i := 0; i < 3; i++ {
			at, reset := base, oldReset
			used, secondary := 80.0, 98.0
			if i > 0 {
				at = base + 1000 + int64(i)*10
				reset = base + 1000 + 300*60
				used, secondary = float64(i-1), float64(i-1)
			}
			secondaryReset := oldReset
			if i > 0 {
				secondaryReset = base + 1000 + 10080*60
			}
			auditInsert(t, s, event{Account: "scoped", QuotaScope: scope, RequestedAt: at, ObservedAt: at, TotalTokens: 100,
				UsedPercent: &used, ResetAt: reset, WindowMinutes: 300, SecondaryUsedPercent: &secondary,
				SecondaryResetAt: secondaryReset, SecondaryWindowMinutes: 10080, PlanType: "plus"})
		}
	}
	if err := s.cleanup(ctx, 365); err != nil {
		t.Fatal(err)
	}
	report, err := s.accounting(ctx, "scoped", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Lifetime.TotalTokens != 600 {
		t.Fatalf("Spark double counted: %+v", report.Lifetime)
	}
	for _, scope := range []string{mainQuotaScope, weeklyQuotaScope, sparkQuotaScope, sparkWeeklyQuotaScope} {
		var n int
		var tokens int64
		for _, r := range report.Resets {
			if r.Scope == scope {
				n++
				tokens += r.Totals.TotalTokens
			}
		}
		if n != 2 || tokens != 300 {
			t.Fatalf("scope %s lost history: cycles=%d tokens=%d", scope, n, tokens)
		}
	}
	for _, q := range report.LifetimeQuota {
		if q.Percent != 1 {
			t.Fatalf("first 0%% reset observation was lost from accounting: %+v", q)
		}
	}
}

func TestAccountingResetHistoryAfterExpiry(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	base := time.Now().Add(-400 * 24 * time.Hour).Unix()
	for i, used := range []float64{80, 0, 1} {
		at := base + int64(i)*100
		reset := base + 50
		if i > 0 {
			reset = base + 100 + 10080*60
		}
		auditInsert(t, s, event{Account: "reset", RequestedAt: at, ObservedAt: at, TotalTokens: 100,
			UsedPercent: &used, ResetAt: reset, WindowMinutes: 10080, PlanType: "pro"})
	}
	before, err := s.accounting(ctx, "reset", 50, 0)
	if err != nil || len(before.Resets) != 2 {
		t.Fatalf("before=%+v err=%v", before, err)
	}
	if err = s.cleanup(ctx, 365); err != nil {
		t.Fatal(err)
	}
	after, err := s.accounting(ctx, "reset", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, r := range after.Resets {
		total += r.Totals.TotalTokens
	}
	if len(after.Resets) != 2 || total != 300 || after.Lifetime.TotalTokens != 300 {
		t.Fatalf("reset ledger lost after cleanup: %+v", after)
	}
}

func TestAccountingPaymentValidationAndAccountIsolation(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for _, p := range []subscriptionPayment{{PaidAt: now}, {Account: "a", PaidAt: 0}, {Account: "a", PaidAt: now + 86400}, {Account: "a", PaidAt: now, Amount: -1}} {
		if s.savePayment(ctx, p) == nil {
			t.Fatalf("accepted invalid %+v", p)
		}
	}
	if err := s.savePayment(ctx, subscriptionPayment{Account: "a", PaidAt: now, Amount: 20}); err != nil {
		t.Fatal(err)
	}
	if s.savePayment(ctx, subscriptionPayment{Account: "a", PaidAt: now}) == nil {
		t.Fatal("accepted duplicate timestamp")
	}
	report, _ := s.accounting(ctx, "a", 50, 0)
	id := report.Subscriptions[0].ID
	if s.deletePayment(ctx, "b", id) == nil || s.savePayment(ctx, subscriptionPayment{ID: id, Account: "b", PaidAt: now}) == nil {
		t.Fatal("cross-account payment modified")
	}
	if err := s.savePayment(ctx, subscriptionPayment{ID: id, Account: "a", PaidAt: now - 100, Amount: 30}); err != nil {
		t.Fatal(err)
	}
	if err := s.deletePayment(ctx, "a", id); err != nil {
		t.Fatal(err)
	}
	report, _ = s.accounting(ctx, "a", 50, 0)
	if len(report.Subscriptions) != 0 {
		t.Fatal("delete did not persist")
	}
}

func TestAccountingManagementRoutes(t *testing.T) {
	s := auditStore(t)
	a := &app{store: s, cfg: defaultConfig()}
	for _, route := range []string{"/accounting", "/subscriptions"} {
		response := a.handleManagement(managementRequest{Method: "PATCH", Path: "/" + pluginID + route, Query: url.Values{"account": {"a"}}})
		if route == "/accounting" && response.StatusCode != 405 {
			t.Fatalf("method status=%d", response.StatusCode)
		}
	}
	payment, _ := json.Marshal(subscriptionPayment{Account: "a", PaidAt: time.Now().Unix(), Amount: 20})
	response := a.handleManagement(managementRequest{Method: "POST", Path: "/" + pluginID + "/subscriptions", Body: payment})
	if response.StatusCode != 200 {
		t.Fatalf("save: %s", response.Body)
	}
	response = a.handleManagement(managementRequest{Method: "GET", Path: "/" + pluginID + "/accounting", Query: url.Values{"account": {"a"}}})
	if response.StatusCode != 200 {
		t.Fatalf("accounting: %s", response.Body)
	}
}

func TestAccountingCrashHelper(t *testing.T) {
	path := os.Getenv("CPA_ACCOUNTING_CRASH_DB")
	if path == "" {
		return
	}
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.insertEvent(context.Background(), event{Account: "crash", IngestID: "committed", RequestedAt: 100, TotalTokens: 77}, time.Minute); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO usage_events(requested_at,account,total_tokens) VALUES(200,'uncommitted',999)`); err != nil {
		t.Fatal(err)
	}
	fmt.Println("CRASH_READY")
	select {}
}

func TestAccountingAbruptProcessDeathAndWALRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.sqlite")
	cmd := exec.Command(os.Args[0], "-test.run=^TestAccountingCrashHelper$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "CPA_ACCOUNTING_CRASH_DB="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "CRASH_READY") {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child never committed")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("child startup timeout")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	var integrity string
	if err = s.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("recovery=%s %v", integrity, err)
	}
	totals, err := s.lifetimeAccounting(context.Background(), "crash")
	if err != nil || totals.TotalTokens != 77 || totals.Requests != 1 {
		t.Fatalf("committed totals=%+v err=%v", totals, err)
	}
	var pending int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE account='uncommitted'`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("uncommitted data survived: %d %v", pending, err)
	}
}

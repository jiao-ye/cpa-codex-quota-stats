package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var accountTables = []string{"quota_samples", "usage_events", "usage_ledger", "quota_cycles", "subscription_payments", "lifetime_ledger"}

func seedDeleteAccount(t *testing.T, s *store, account string) {
	t.Helper()
	used := 10.0
	auditInsert(t, s, event{Account: account, IngestID: account + "-old", RequestedAt: 100, ObservedAt: 100,
		TotalTokens: 42, UsedPercent: &used, ResetAt: 18100, WindowMinutes: 300})
	if err := s.savePayment(context.Background(), subscriptionPayment{Account: account, PaidAt: 100, Amount: 20}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAccountPreservesOthersAndDoesNotReturnAfterMigration(t *testing.T) {
	s := auditStore(t)
	ctx := context.Background()
	for _, account := range []string{"remove", "keep", "quotes' OR 1=1 --"} {
		seedDeleteAccount(t, s, account)
	}
	for _, account := range []string{"remove", "quotes' OR 1=1 --"} {
		if err := s.deleteAccount(ctx, account); err != nil {
			t.Fatal(err)
		}
		for _, table := range accountTables {
			var count int
			if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE account=?", account).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s not cleared: %d %v", table, count, err)
			}
		}
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	accounts, err := s.accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0] != "keep" {
		t.Fatalf("accounts=%v err=%v", accounts, err)
	}
	for _, table := range accountTables {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE account=?", "keep").Scan(&count); err != nil || count != 1 {
			t.Fatalf("other account changed in %s: %d %v", table, count, err)
		}
	}
	auditInsert(t, s, event{Account: "remove", IngestID: "remove-new", RequestedAt: 200, TotalTokens: 7})
	total, err := s.lifetimeAccounting(ctx, "remove")
	if err != nil || total.Requests != 1 || total.TotalTokens != 7 {
		t.Fatalf("new usage did not start fresh: %+v %v", total, err)
	}
}

func TestDeleteAccountRollsBackAllTablesOnFailure(t *testing.T) {
	s := auditStore(t)
	seedDeleteAccount(t, s, "remove")
	if _, err := s.db.Exec(`CREATE TRIGGER reject_account_delete BEFORE DELETE ON lifetime_ledger
BEGIN SELECT RAISE(ABORT,'synthetic failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := s.deleteAccount(context.Background(), "remove"); err == nil {
		t.Fatal("expected transaction failure")
	}
	for _, table := range accountTables {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE account=?", "remove").Scan(&count); err != nil || count != 1 {
			t.Fatalf("partial deletion in %s: %d %v", table, count, err)
		}
	}
}

func TestDeleteAccountRouteRequiresConfirmation(t *testing.T) {
	s := auditStore(t)
	seedDeleteAccount(t, s, "remove")
	a := &app{store: s, cfg: defaultConfig()}
	path := "/v0/management/" + pluginID + "/accounts"
	for _, body := range []string{"{}", `{"account":"remove"}`, `{"account":"","confirm":true}`, `{"account":"  ","confirm":true}`, `not-json`} {
		response := a.handleManagement(managementRequest{Method: "DELETE", Path: path, Body: []byte(body)})
		if response.StatusCode != 400 {
			t.Fatalf("unconfirmed deletion accepted: %d", response.StatusCode)
		}
	}
	if response := a.handleManagement(managementRequest{Method: "GET", Path: path}); response.StatusCode != 405 {
		t.Fatal(response.StatusCode)
	}
	before, err := s.lifetimeAccounting(context.Background(), "remove")
	if err != nil || before.TotalTokens != 42 {
		t.Fatalf("invalid requests changed totals: %+v %v", before, err)
	}
	body, _ := json.Marshal(map[string]any{"account": "remove", "confirm": true})
	response := a.handleManagement(managementRequest{Method: "DELETE", Path: path, Body: body})
	if response.StatusCode != 200 || !strings.Contains(string(response.Body), `"ok":true`) {
		t.Fatalf("confirmed deletion failed: %d", response.StatusCode)
	}
	if err := s.insertEvent(context.Background(), event{Account: "remove", RequestedAt: 200, TotalTokens: 7}, time.Minute); err != nil {
		t.Fatal(err)
	}
}

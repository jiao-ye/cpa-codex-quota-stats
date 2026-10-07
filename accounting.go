package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Tokens and rate snapshots are immutable, independent of raw detail retention.
type ledgerTotals struct {
	Requests        int64   `json:"requests"`
	Failed          int64   `json:"failed"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	Reasoning       int64   `json:"reasoning_tokens"`
	CacheRead       int64   `json:"cache_read_tokens"`
	CacheWrite      int64   `json:"cache_write_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	USDReference    float64 `json:"usd_reference"`
	CreditReference float64 `json:"credits_reference"`
	USDUnpriced     int64   `json:"usd_unpriced_requests"`
	CreditUnpriced  int64   `json:"credits_unpriced_requests"`
	FirstAt         int64   `json:"first_request_at"`
	LastAt          int64   `json:"last_request_at"`
}

type subscriptionPayment struct {
	ID      int64   `json:"id"`
	Account string  `json:"account"`
	PaidAt  int64   `json:"paid_at"`
	Amount  float64 `json:"amount_usd"`
}

type subscriptionLedger struct {
	subscriptionPayment
	NextPaidAt int64             `json:"next_paid_at"`
	Totals     ledgerTotals      `json:"totals"`
	Partial    bool              `json:"partial"`
	Quota      []quotaAccounting `json:"observed_quota_growth"`
}

type quotaAccounting struct {
	Scope   string  `json:"scope"`
	Percent float64 `json:"percent"`
	Partial bool    `json:"partial"`
}

type resetLedger struct {
	Scope string `json:"scope"`
	quotaCycle
	Totals ledgerTotals `json:"totals"`
}

type accountingResponse struct {
	Account       string               `json:"account"`
	RecordedSince int64                `json:"recorded_since"`
	Partial       bool                 `json:"historical_coverage_partial"`
	Dropped       int64                `json:"dropped_usage_events"`
	Lifetime      ledgerTotals         `json:"lifetime"`
	LifetimeQuota []quotaAccounting    `json:"lifetime_observed_quota_growth"`
	Subscriptions []subscriptionLedger `json:"subscriptions"`
	Resets        []resetLedger        `json:"resets"`
	Unassigned    ledgerTotals         `json:"unassigned_main"`
	Observations  []quotaObservation   `json:"last_quota_observations"`
}

func (s *store) migrateAccounting() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS usage_ledger (
 id INTEGER PRIMARY KEY,
 ingest_id TEXT NOT NULL DEFAULT '',
 cycle_id INTEGER NOT NULL DEFAULT 0,
 requested_at INTEGER NOT NULL,
 observed_at INTEGER NOT NULL,
 account TEXT NOT NULL,
 model TEXT NOT NULL,
 service_tier TEXT NOT NULL,
 quota_scope TEXT NOT NULL,
 input_tokens INTEGER NOT NULL,
 output_tokens INTEGER NOT NULL,
 reasoning_tokens INTEGER NOT NULL,
 cache_read_tokens INTEGER NOT NULL,
 cache_write_tokens INTEGER NOT NULL,
 total_tokens INTEGER NOT NULL,
 failed INTEGER NOT NULL,
 used_percent REAL,
 reset_at INTEGER NOT NULL,
 window_minutes INTEGER NOT NULL,
 secondary_used_percent REAL,
 secondary_reset_at INTEGER NOT NULL,
 secondary_window_minutes INTEGER NOT NULL,
 plan_type TEXT NOT NULL,
 selected_value REAL NOT NULL,
 usd_reference REAL,
 credits_reference REAL,
 rate_snapshot TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_ingest ON usage_ledger(ingest_id) WHERE ingest_id<>'';
CREATE INDEX IF NOT EXISTS idx_ledger_account_time ON usage_ledger(account,requested_at,id);
CREATE INDEX IF NOT EXISTS idx_ledger_cycle ON usage_ledger(cycle_id,quota_scope,requested_at);
CREATE INDEX IF NOT EXISTS idx_ledger_observed ON usage_ledger(account,quota_scope,observed_at,id);
DROP VIEW IF EXISTS quota_history;
CREATE VIEW quota_history AS
 SELECT id,cycle_id,requested_at,observed_at,account,model,service_tier,quota_scope,total_tokens,selected_value AS cost_usd,failed,
 used_percent,reset_at,window_minutes,secondary_used_percent,secondary_reset_at,secondary_window_minutes,plan_type,
 input_tokens,output_tokens,cache_read_tokens,cache_write_tokens FROM usage_ledger
 UNION ALL
 SELECT id,cycle_id,requested_at,CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END,account,model,service_tier,quota_scope,total_tokens,cost_usd,failed,
 used_percent,reset_at,window_minutes,secondary_used_percent,secondary_reset_at,secondary_window_minutes,plan_type,
 input_tokens,output_tokens,cache_read_tokens,cache_write_tokens FROM usage_events
 WHERE id NOT IN (SELECT id FROM usage_ledger);
CREATE TABLE IF NOT EXISTS lifetime_ledger (
 account TEXT NOT NULL,
 quota_scope TEXT NOT NULL,
 requests INTEGER NOT NULL DEFAULT 0,
 failed INTEGER NOT NULL DEFAULT 0,
 input_tokens INTEGER NOT NULL DEFAULT 0,
 output_tokens INTEGER NOT NULL DEFAULT 0,
 reasoning_tokens INTEGER NOT NULL DEFAULT 0,
 cache_read_tokens INTEGER NOT NULL DEFAULT 0,
 cache_write_tokens INTEGER NOT NULL DEFAULT 0,
 total_tokens INTEGER NOT NULL DEFAULT 0,
 usd_reference REAL NOT NULL DEFAULT 0,
 credits_reference REAL NOT NULL DEFAULT 0,
 usd_unpriced INTEGER NOT NULL DEFAULT 0,
 credits_unpriced INTEGER NOT NULL DEFAULT 0,
 first_at INTEGER NOT NULL,
 last_at INTEGER NOT NULL,
 PRIMARY KEY(account,quota_scope)
);
CREATE TABLE IF NOT EXISTS subscription_payments (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 account TEXT NOT NULL,
 paid_at INTEGER NOT NULL CHECK(paid_at>0),
 amount_usd REAL NOT NULL CHECK(amount_usd>=0),
 UNIQUE(account,paid_at)
);
CREATE TRIGGER IF NOT EXISTS ledger_cycle_update AFTER UPDATE OF cycle_id,cost_usd ON usage_events BEGIN
 UPDATE usage_ledger SET cycle_id=NEW.cycle_id,selected_value=NEW.cost_usd WHERE id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS ledger_lifetime_insert AFTER INSERT ON usage_ledger BEGIN
 INSERT INTO lifetime_ledger(account,quota_scope,requests,failed,input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,total_tokens,usd_reference,credits_reference,usd_unpriced,credits_unpriced,first_at,last_at)
 VALUES(NEW.account,NEW.quota_scope,1,NEW.failed,NEW.input_tokens,NEW.output_tokens,NEW.reasoning_tokens,NEW.cache_read_tokens,NEW.cache_write_tokens,NEW.total_tokens,COALESCE(NEW.usd_reference,0),COALESCE(NEW.credits_reference,0),NEW.usd_reference IS NULL,NEW.credits_reference IS NULL,NEW.requested_at,NEW.requested_at)
 ON CONFLICT(account,quota_scope) DO UPDATE SET
 requests=requests+1,failed=failed+NEW.failed,input_tokens=input_tokens+NEW.input_tokens,output_tokens=output_tokens+NEW.output_tokens,
 reasoning_tokens=reasoning_tokens+NEW.reasoning_tokens,cache_read_tokens=cache_read_tokens+NEW.cache_read_tokens,cache_write_tokens=cache_write_tokens+NEW.cache_write_tokens,
 total_tokens=total_tokens+NEW.total_tokens,usd_reference=usd_reference+COALESCE(NEW.usd_reference,0),credits_reference=credits_reference+COALESCE(NEW.credits_reference,0),
 usd_unpriced=usd_unpriced+(NEW.usd_reference IS NULL),credits_unpriced=credits_unpriced+(NEW.credits_reference IS NULL),
 first_at=MIN(first_at,NEW.requested_at),last_at=MAX(last_at,NEW.requested_at);
END;
INSERT OR IGNORE INTO metadata(key,value) VALUES('accounting_started_at',strftime('%s','now'));
`)
	if err != nil {
		return err
	}
	// Restartable migration: each batch and its permanent totals commit together.
	for {
		rows, queryErr := s.db.Query(`SELECT id,ingest_id,cycle_id,requested_at,observed_at,account,model,service_tier,quota_scope,
input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,total_tokens,failed,
used_percent,reset_at,window_minutes,secondary_used_percent,secondary_reset_at,secondary_window_minutes,plan_type,cost_usd
FROM usage_events WHERE id NOT IN (SELECT id FROM usage_ledger) ORDER BY id LIMIT 500`)
		if queryErr != nil {
			return queryErr
		}
		type retained struct {
			id, cycle int64
			e         event
		}
		batch := make([]retained, 0, 500)
		for rows.Next() {
			var r retained
			e := &r.e
			var used, secondary sql.NullFloat64
			if err = rows.Scan(&r.id, &e.IngestID, &r.cycle, &e.RequestedAt, &e.ObservedAt, &e.Account, &e.Model, &e.ServiceTier, &e.QuotaScope,
				&e.InputTokens, &e.OutputTokens, &e.ReasoningTokens, &e.CacheReadTokens, &e.CacheWriteTokens, &e.TotalTokens, &e.Failed,
				&used, &e.ResetAt, &e.WindowMinutes, &secondary, &e.SecondaryResetAt, &e.SecondaryWindowMinutes, &e.PlanType, &e.CostUSD); err != nil {
				rows.Close()
				return err
			}
			if used.Valid {
				e.UsedPercent = &used.Float64
			}
			if secondary.Valid {
				e.SecondaryUsedPercent = &secondary.Float64
			}
			batch = append(batch, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || len(batch) == 0 {
			return err
		}
		tx, beginErr := s.db.Begin()
		if beginErr != nil {
			return beginErr
		}
		for _, r := range batch {
			if err = s.insertAccounting(context.Background(), tx, r.id, r.cycle, r.e, true); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
}

func (s *store) insertAccounting(ctx context.Context, tx *sql.Tx, id, cycle int64, e event, backfill bool) error {
	credits, creditsOK := officialCreditsForUsage(e.Model, e.ServiceTier, e.InputTokens, e.CacheReadTokens, e.CacheWriteTokens, e.OutputTokens)
	var creditValue, usdValue any
	if creditsOK {
		creditValue = credits
	}
	var p price
	priceErr := tx.QueryRowContext(ctx, `SELECT model,input,output,cache_read,cache_write,long_input,long_output,long_cache_read,long_cache_write,fast_input,fast_output,fast_cache_read,fast_cache_write,source,updated_at FROM model_prices WHERE lower(model)=? LIMIT 1`, normalizeModel(e.Model)).
		Scan(&p.Model, &p.Input, &p.Output, &p.CacheRead, &p.CacheWrite, &p.LongInput, &p.LongOutput, &p.LongRead, &p.LongWrite, &p.FastInput, &p.FastOutput, &p.FastRead, &p.FastWrite, &p.Source, &p.UpdatedAt)
	if priceErr != nil && priceErr != sql.ErrNoRows {
		return priceErr
	}
	if priceErr == nil {
		cfg := defaultConfig()
		cfg.PricingMode = pricingModeAPI
		usdValue = calculateCost(p, usageDetail{InputTokens: e.InputTokens, OutputTokens: e.OutputTokens, CacheReadTokens: e.CacheReadTokens, CacheCreationTokens: e.CacheWriteTokens}, e.ServiceTier, cfg)
	}
	creditRate, _ := officialCodexCreditPrice(e.Model)
	snapshot, err := json.Marshal(map[string]any{
		"formula": "upstream-reference-v0.19.4", "api_rate": p, "credit_rate": creditRate, "credit_rate_known": creditsOK, "backfilled": backfill,
		"not_actual_charge": true,
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_ledger(id,ingest_id,cycle_id,requested_at,observed_at,account,model,service_tier,quota_scope,
input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,total_tokens,failed,used_percent,reset_at,window_minutes,
secondary_used_percent,secondary_reset_at,secondary_window_minutes,plan_type,selected_value,usd_reference,credits_reference,rate_snapshot)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, e.IngestID, cycle, e.RequestedAt, eventObservationTime(e), e.Account, e.Model, e.ServiceTier, eventQuotaScope(e),
		e.InputTokens, e.OutputTokens, e.ReasoningTokens, e.CacheReadTokens, e.CacheWriteTokens, e.TotalTokens, e.Failed, e.UsedPercent, e.ResetAt, e.WindowMinutes,
		e.SecondaryUsedPercent, e.SecondaryResetAt, e.SecondaryWindowMinutes, e.PlanType, e.CostUSD, usdValue, creditValue, string(snapshot))
	return err
}

const ledgerAggregate = `COUNT(*),COALESCE(SUM(failed),0),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),
COALESCE(SUM(reasoning_tokens),0),COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(cache_write_tokens),0),COALESCE(SUM(total_tokens),0),
COALESCE(SUM(usd_reference),0),COALESCE(SUM(credits_reference),0),COALESCE(SUM(usd_reference IS NULL),0),COALESCE(SUM(credits_reference IS NULL),0),
COALESCE(MIN(requested_at),0),COALESCE(MAX(requested_at),0)`

func scanLedger(row *sql.Row) (ledgerTotals, error) {
	var t ledgerTotals
	err := row.Scan(&t.Requests, &t.Failed, &t.InputTokens, &t.OutputTokens, &t.Reasoning, &t.CacheRead, &t.CacheWrite, &t.TotalTokens,
		&t.USDReference, &t.CreditReference, &t.USDUnpriced, &t.CreditUnpriced, &t.FirstAt, &t.LastAt)
	return t, err
}

func (s *store) ledgerRange(ctx context.Context, account string, start, end int64, filter string, args ...any) (ledgerTotals, error) {
	parameters := []any{account, start, end}
	parameters = append(parameters, args...)
	return scanLedger(s.db.QueryRowContext(ctx, `SELECT `+ledgerAggregate+` FROM usage_ledger WHERE account=? AND requested_at>=? AND requested_at<? `+filter, parameters...))
}

func (s *store) lifetimeAccounting(ctx context.Context, account string) (ledgerTotals, error) {
	return scanLedger(s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(requests),0),COALESCE(SUM(failed),0),
COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(reasoning_tokens),0),COALESCE(SUM(cache_read_tokens),0),
COALESCE(SUM(cache_write_tokens),0),COALESCE(SUM(total_tokens),0),COALESCE(SUM(usd_reference),0),COALESCE(SUM(credits_reference),0),
COALESCE(SUM(usd_unpriced),0),COALESCE(SUM(credits_unpriced),0),COALESCE(MIN(first_at),0),COALESCE(MAX(last_at),0)
FROM lifetime_ledger WHERE account=?`, account))
}

func (s *store) savePayment(ctx context.Context, payment subscriptionPayment) error {
	if payment.Account == "" || payment.PaidAt <= 0 || payment.PaidAt > time.Now().Unix()+300 ||
		payment.Amount < 0 || math.IsNaN(payment.Amount) || math.IsInf(payment.Amount, 0) {
		return fmt.Errorf("account, actual past payment time, and nonnegative USD amount are required")
	}
	if payment.ID > 0 {
		result, err := s.db.ExecContext(ctx, `UPDATE subscription_payments SET paid_at=?,amount_usd=? WHERE id=? AND account=?`, payment.PaidAt, payment.Amount, payment.ID, payment.Account)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n == 0 {
			return fmt.Errorf("payment not found for this account")
		}
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO subscription_payments(account,paid_at,amount_usd) VALUES(?,?,?)`, payment.Account, payment.PaidAt, payment.Amount)
	return err
}

func (s *store) deletePayment(ctx context.Context, account string, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM subscription_payments WHERE account=? AND id=?`, account, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return fmt.Errorf("payment not found for this account")
	}
	return err
}

func (s *store) accounting(ctx context.Context, account string, limit, offset int) (accountingResponse, error) {
	result := accountingResponse{Account: account, Partial: true, Subscriptions: []subscriptionLedger{}, Resets: []resetLedger{}}
	var err error
	result.Lifetime, err = s.lifetimeAccounting(ctx, account)
	if err != nil {
		return result, err
	}
	result.RecordedSince = result.Lifetime.FirstAt
	rows, err := s.db.QueryContext(ctx, `SELECT id,account,paid_at,amount_usd,
LEAD(paid_at,1,0) OVER(ORDER BY paid_at) FROM subscription_payments WHERE account=? ORDER BY paid_at`, account)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var period subscriptionLedger
		if err = rows.Scan(&period.ID, &period.Account, &period.PaidAt, &period.Amount, &period.NextPaidAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Subscriptions = append(result.Subscriptions, period)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for i := range result.Subscriptions {
		p := &result.Subscriptions[i]
		end := p.NextPaidAt
		if end == 0 {
			end = math.MaxInt64
		}
		p.Totals, err = s.ledgerRange(ctx, account, p.PaidAt, end, "")
		if err != nil {
			return result, err
		}
		p.Partial = result.RecordedSince == 0 || p.PaidAt < result.RecordedSince
		p.Quota, err = s.accountingQuotaGrowth(ctx, account, p.PaidAt, end)
		if err != nil {
			return result, err
		}
	}
	result.LifetimeQuota, err = s.accountingQuotaGrowth(ctx, account, 0, math.MaxInt64)
	if err != nil {
		return result, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	// Unlike chart selectors this ledger supports pagination of all history.
	rows, err = s.db.QueryContext(ctx, `SELECT id,started_at,ended_at,reset_at,window_minutes,plan_type,close_reason,
first_sample_at,last_sample_at,start_used_percent,end_used_percent,peak_used_percent FROM quota_cycles
WHERE account=? ORDER BY started_at DESC,id DESC LIMIT ? OFFSET ?`, account, limit, offset)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		r := resetLedger{Scope: mainQuotaScope}
		if err = rows.Scan(&r.ID, &r.StartedAt, &r.EndedAt, &r.ResetAt, &r.WindowMinutes, &r.PlanType, &r.CloseReason,
			&r.FirstSampleAt, &r.LastSampleAt, &r.StartPercent, &r.EndPercent, &r.PeakPercent); err != nil {
			rows.Close()
			return result, err
		}
		r.Current = r.EndedAt == 0
		result.Resets = append(result.Resets, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for i := range result.Resets {
		r := &result.Resets[i]
		r.Totals, err = s.ledgerRange(ctx, account, 0, math.MaxInt64, "AND quota_scope=? AND cycle_id=?", mainQuotaScope, r.ID)
		if err != nil {
			return result, err
		}
	}
	for _, scope := range []string{weeklyQuotaScope, sparkQuotaScope, sparkWeeklyQuotaScope} {
		cycles, scopeErr := s.quotaScopeCycles(ctx, account, scope)
		if scopeErr != nil {
			return result, scopeErr
		}
		source := quotaScopeSource(scope)
		for i, c := range cycles {
			if i < offset || i >= offset+limit {
				continue
			}
			r := resetLedger{Scope: scope, quotaCycle: c}
			// Scope membership is established by response observation time.
			r.Totals, err = scanLedger(s.db.QueryRowContext(ctx, `SELECT `+ledgerAggregate+` FROM usage_ledger
WHERE account=? AND quota_scope=? AND `+source.EventFilter+` AND observed_at>=? AND observed_at<?`,
				account, source.EventScope, c.StartedAt, scopedCycleEnd(c)))
			if err != nil {
				return result, err
			}
			result.Resets = append(result.Resets, r)
		}
	}
	result.Unassigned, err = s.ledgerRange(ctx, account, 0, math.MaxInt64, "AND quota_scope=? AND cycle_id=0", mainQuotaScope)
	if err == nil {
		result.Observations, err = s.lastQuotaObservations(ctx, account)
	}
	return result, err
}

func (s *store) accountingQuotaGrowth(ctx context.Context, account string, start, end int64) ([]quotaAccounting, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,started_at,ended_at,reset_at,window_minutes,plan_type FROM quota_cycles
WHERE account=? AND started_at<? AND (ended_at=0 OR ended_at>?) ORDER BY started_at,id`, account, end, start)
	if err != nil {
		return nil, err
	}
	var mainCycles []quotaCycle
	for rows.Next() {
		var c quotaCycle
		if err = rows.Scan(&c.ID, &c.StartedAt, &c.EndedAt, &c.ResetAt, &c.WindowMinutes, &c.PlanType); err != nil {
			rows.Close()
			return nil, err
		}
		mainCycles = append(mainCycles, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []quotaAccounting{}
	for _, scope := range []string{mainQuotaScope, weeklyQuotaScope, sparkQuotaScope, sparkWeeklyQuotaScope} {
		cycles := mainCycles
		if scope != mainQuotaScope {
			cycles, err = s.quotaScopeCycles(ctx, account, scope)
			if err != nil {
				return nil, err
			}
		}
		if len(cycles) == 0 {
			continue
		}
		q := quotaAccounting{Scope: scope}
		for _, c := range cycles {
			if c.StartedAt >= end || c.EndedAt > 0 && c.EndedAt <= start {
				continue
			}
			var growth float64
			var complete bool
			if scope == mainQuotaScope {
				growth, complete, err = s.cycleGrowth(ctx, c, start, end, false)
			} else {
				growth, complete, err = s.scopedCycleGrowth(ctx, account, scope, c, start, end, false)
			}
			if err != nil {
				return nil, err
			}
			q.Percent += growth
			q.Partial = q.Partial || !complete
		}
		result = append(result, q)
	}
	return result, nil
}

package main

import (
	"context"
	"database/sql"
)

type actualUsageRow struct {
	Model  string       `json:"model"`
	Tier   string       `json:"service_tier"`
	Totals ledgerTotals `json:"totals"`
}

func (s *store) actualUsageBreakdown(ctx context.Context, account string, start, end int64) ([]actualUsageRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT model,service_tier,`+ledgerAggregate+`
FROM usage_ledger WHERE account=? AND requested_at>=? AND requested_at<?
GROUP BY model,service_tier ORDER BY SUM(total_tokens) DESC,model,service_tier`, account, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []actualUsageRow{}
	for rows.Next() {
		var r actualUsageRow
		t := &r.Totals
		if err = rows.Scan(&r.Model, &r.Tier, &t.Requests, &t.Failed, &t.InputTokens, &t.OutputTokens, &t.Reasoning, &t.CacheRead, &t.CacheWrite,
			&t.TotalTokens, &t.USDReference, &t.CreditReference, &t.USDUnpriced, &t.CreditUnpriced, &t.FirstAt, &t.LastAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type quotaObservation struct {
	Scope   string  `json:"scope"`
	Used    float64 `json:"used_percent"`
	At      int64   `json:"observed_at"`
	ResetAt int64   `json:"reset_at"`
	Window  int64   `json:"window_minutes"`
}

func (s *store) lastQuotaObservations(ctx context.Context, account string) ([]quotaObservation, error) {
	result := []quotaObservation{}
	for _, scope := range []string{mainQuotaScope, weeklyQuotaScope, sparkQuotaScope, sparkWeeklyQuotaScope} {
		source := quotaScopeSource(scope)
		var q quotaObservation
		q.Scope = scope
		err := s.db.QueryRowContext(ctx, `SELECT `+source.UsedExpr+`,observed_at,`+source.ResetExpr+`,`+source.WindowExpr+`
FROM usage_ledger WHERE account=? AND quota_scope=? AND `+source.EventFilter+`
AND failed=0 AND `+source.UsedExpr+` IS NOT NULL AND `+source.ResetExpr+`>0
ORDER BY observed_at DESC,id DESC LIMIT 1`, account, source.EventScope).Scan(&q.Used, &q.At, &q.ResetAt, &q.Window)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, q)
	}
	return result, nil
}

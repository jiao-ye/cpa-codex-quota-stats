package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *store) months(ctx context.Context, account string, limit int) ([]string, error) {
	if limit <= 0 || limit > 120 {
		limit = 36
	}
	var firstEvent, firstCycle int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(requested_at),0) FROM quota_history WHERE account=?`, account).Scan(&firstEvent); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(started_at),0) FROM quota_cycles WHERE account=?`, account).Scan(&firstCycle); err != nil {
		return nil, err
	}
	earliest := firstEvent
	if earliest == 0 || firstCycle > 0 && firstCycle < earliest {
		earliest = firstCycle
	}
	location := shanghaiLocation()
	now := time.Now().In(location)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	if earliest == 0 {
		return []string{month.Format("2006-01")}, nil
	}
	first := time.Unix(earliest, 0).In(location)
	firstMonth := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, location)
	months := make([]string, 0, limit)
	for len(months) < limit && !month.Before(firstMonth) {
		months = append(months, month.Format("2006-01"))
		month = month.AddDate(0, -1, 0)
	}
	return months, nil
}

func monthRange(raw string) (string, int64, int64, error) {
	location := shanghaiLocation()
	if raw == "" {
		now := time.Now().In(location)
		raw = fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))
	}
	month, err := time.ParseInLocation("2006-01", raw, location)
	if err != nil || month.Format("2006-01") != raw {
		return "", 0, 0, fmt.Errorf("invalid month %q; expected YYYY-MM", raw)
	}
	return raw, month.Unix(), month.AddDate(0, 1, 0).Unix(), nil
}

type monthlyCycle struct {
	quotaCycle
	MonthTokens   int64   `json:"month_tokens"`
	MonthCostUSD  float64 `json:"month_reference_value"`
	MonthRequests int64   `json:"month_requests"`
}

type monthlySummary struct {
	Month                   string         `json:"month"`
	Timezone                string         `json:"timezone"`
	StartAt                 int64          `json:"start_at"`
	EndAt                   int64          `json:"end_at"`
	ActualTokens            int64          `json:"actual_tokens"`
	ActualCostUSD           float64        `json:"reference_value"`
	Requests                int64          `json:"requests"`
	CycleCount              int            `json:"cycle_count"`
	ResetCount              int            `json:"reset_count"`
	EarlyResetCount         int            `json:"early_reset_count"`
	AllocatedCycleCount     int            `json:"allocated_cycle_count"`
	ConsumedQuotaPercent    float64        `json:"consumed_quota_percent"`
	ConsumedQuotaEquivalent float64        `json:"consumed_quota_equivalent"`
	QuotaCoverageComplete   bool           `json:"quota_coverage_complete"`
	UnusedQuotaAtReset      float64        `json:"unused_quota_at_reset"`
	Cycles                  []monthlyCycle `json:"cycles"`
}

func (s *store) monthly(ctx context.Context, account, month string) (monthlySummary, error) {
	return s.monthlyObserved(ctx, account, mainQuotaScope, month, time.Now().Unix())
}

func (s *store) monthlyQuotaScope(ctx context.Context, account, scope, month string) (monthlySummary, error) {
	return s.monthlyObserved(ctx, account, scope, month, time.Now().Unix())
}

func (s *store) monthlyQuotaScopeAt(ctx context.Context, account, scope, month string, now int64) (monthlySummary, error) {
	return s.monthlyObserved(ctx, account, scope, month, now)
}

func (s *store) monthlyObserved(ctx context.Context, account, scope, raw string, now int64) (monthlySummary, error) {
	month, start, end, err := monthRange(raw)
	if err != nil {
		return monthlySummary{}, err
	}
	result := monthlySummary{Month: month, Timezone: "Asia/Shanghai", StartAt: start, EndAt: end, QuotaCoverageComplete: true, Cycles: []monthlyCycle{}}
	source := quotaScopeSource(scope)
	if err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(total_tokens),0),COALESCE(SUM(cost_usd),0),COUNT(*)
FROM quota_history WHERE account=? AND quota_scope=? AND `+source.EventFilter+` AND requested_at>=? AND requested_at<?`, account, source.EventScope, start, end).
		Scan(&result.ActualTokens, &result.ActualCostUSD, &result.Requests); err != nil {
		return result, err
	}
	var cycles []quotaCycle
	if scope == mainQuotaScope {
		cycles, err = s.cycles(ctx, account, 1000)
	} else {
		cycles, err = s.quotaScopeCycles(ctx, account, scope)
	}
	if err != nil {
		return result, err
	}
	for _, c := range cycles {
		cycleEnd := scopedCycleEnd(c)
		if c.Current && c.ResetAt > now {
			cycleEnd = now + 1
		}
		if c.StartedAt >= end || cycleEnd <= start {
			continue
		}
		item := monthlyCycle{quotaCycle: c}
		if scope == mainQuotaScope {
			err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(total_tokens),0),COALESCE(SUM(cost_usd),0),COUNT(*)
FROM quota_history WHERE cycle_id=? AND quota_scope=? AND requested_at>=? AND requested_at<?`, c.ID, scope, start, end).
				Scan(&item.MonthTokens, &item.MonthCostUSD, &item.MonthRequests)
		} else {
			err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(total_tokens),0),COALESCE(SUM(cost_usd),0),COUNT(*)
FROM quota_history WHERE account=? AND quota_scope=? AND `+source.EventFilter+` AND requested_at>=? AND requested_at<? AND requested_at>=? AND requested_at<?`,
				account, source.EventScope, c.StartedAt, scopedCycleEnd(c), start, end).Scan(&item.MonthTokens, &item.MonthCostUSD, &item.MonthRequests)
		}
		if err != nil {
			return result, err
		}
		result.Cycles = append(result.Cycles, item)
		result.CycleCount++
		var growth float64
		var complete bool
		if scope == mainQuotaScope {
			growth, complete, err = s.cycleQuotaGrowth(ctx, c, start, end)
		} else {
			growth, complete, err = s.quotaScopeCycleGrowth(ctx, account, scope, c, start, end)
		}
		if err != nil {
			return result, err
		}
		result.ConsumedQuotaPercent += growth
		result.QuotaCoverageComplete = result.QuotaCoverageComplete && complete
		if c.EndedAt >= start && c.EndedAt < end && isResetCloseReason(c.CloseReason) {
			result.ResetCount++
			if c.CloseReason == "early_reset" {
				result.EarlyResetCount++
			}
			result.UnusedQuotaAtReset += maxFloat(0, 100-c.PeakPercent)
		}
		if c.StartedAt >= start && c.StartedAt < end {
			result.AllocatedCycleCount++
		}
	}
	result.ConsumedQuotaEquivalent = result.ConsumedQuotaPercent / 100
	return result, nil
}

func isResetCloseReason(reason string) bool {
	return reason == "scheduled_reset" || reason == "early_reset" || reason == "observed_reset"
}

func (s *store) cycleQuotaGrowth(ctx context.Context, cycle quotaCycle, startAt, endAt int64) (float64, bool, error) {
	return s.cycleGrowth(ctx, cycle, startAt, endAt, true)
}

func (s *store) cycleGrowth(ctx context.Context, cycle quotaCycle, startAt, endAt int64, resetBaseline bool) (float64, bool, error) {
	anomalies, err := s.quotaRegimeAnomalies(ctx, cycle.ID)
	if err != nil {
		return 0, false, err
	}
	// Use the same confirmed-regime exclusion as the charts, without their
	// display point limit truncating a busy account's accounting totals.
	filter := ""
	table, pointTime := "quota_samples", "sampled_at"
	if !resetBaseline {
		table, pointTime = "quota_history", "requested_at"
		filter = " AND quota_scope='main' AND failed=0 AND used_percent IS NOT NULL"
	}
	args := []any{cycle.ID}
	for _, anomaly := range anomalies {
		filter += " AND NOT (" + pointTime + ">=? AND " + pointTime + "<? AND reset_at=?)"
		args = append(args, anomaly.StartedAt, anomaly.EndedAt, anomaly.AnomalousResetAt)
	}
	peakBefore := func(at int64) (sql.NullFloat64, error) {
		var value sql.NullFloat64
		queryArgs := append(append([]any{}, args...), at)
		queryErr := s.db.QueryRowContext(ctx, `SELECT MAX(used_percent) FROM `+table+` WHERE cycle_id=?`+filter+` AND `+pointTime+`<?`, queryArgs...).Scan(&value)
		return value, queryErr
	}
	var endPeak sql.NullFloat64
	if endPeak, err = peakBefore(endAt); err != nil {
		return 0, false, err
	}
	if !endPeak.Valid {
		return 0, false, nil
	}
	if resetBaseline && cycle.StartedAt >= startAt {
		// The upstream percentage is cumulative from the cycle reset. Once the
		// reset boundary is known, the peak directly describes consumption in
		// this month even if the first observed request was not near 0%.
		return maxFloat(0, endPeak.Float64), true, nil
	}
	var baseline sql.NullFloat64
	if baseline, err = peakBefore(startAt); err != nil {
		return 0, false, err
	}
	if baseline.Valid {
		return maxFloat(0, endPeak.Float64-baseline.Float64), true, nil
	}
	var firstInMonth sql.NullFloat64
	rangeArgs := append(append([]any{}, args...), startAt, endAt)
	if err := s.db.QueryRowContext(ctx, `SELECT used_percent FROM `+table+` WHERE cycle_id=?`+filter+` AND `+pointTime+`>=? AND `+pointTime+`<? ORDER BY `+pointTime+` ASC,id ASC LIMIT 1`, rangeArgs...).Scan(&firstInMonth); err != nil && err != sql.ErrNoRows {
		return 0, false, err
	}
	if firstInMonth.Valid {
		return maxFloat(0, endPeak.Float64-firstInMonth.Float64), false, nil
	}
	return 0, false, nil
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

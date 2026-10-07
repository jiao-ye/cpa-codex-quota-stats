package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type scopedQuotaSource struct {
	EventScope  string
	EventFilter string
	UsedExpr    string
	ResetExpr   string
	WindowExpr  string
}

func quotaScopeSource(scope string) scopedQuotaSource {
	if scope == weeklyQuotaScope || scope == sparkWeeklyQuotaScope {
		primaryCondition := fmt.Sprintf("window_minutes BETWEEN %d AND %d", fiveHourWindowMinutes-fiveHourWindowSlack, fiveHourWindowMinutes+fiveHourWindowSlack)
		secondaryCondition := fmt.Sprintf("secondary_window_minutes BETWEEN %d AND %d", weeklyWindowMinutes-weeklyWindowSlack, weeklyWindowMinutes+weeklyWindowSlack)
		quotaCondition := primaryCondition + " AND " + secondaryCondition
		eventScope := mainQuotaScope
		if scope == sparkWeeklyQuotaScope {
			eventScope = sparkQuotaScope
		}
		return scopedQuotaSource{
			EventScope:  eventScope,
			EventFilter: primaryCondition,
			UsedExpr:    "CASE WHEN " + quotaCondition + " THEN secondary_used_percent END",
			ResetExpr:   "CASE WHEN " + quotaCondition + " THEN secondary_reset_at ELSE 0 END",
			WindowExpr:  "CASE WHEN " + quotaCondition + " THEN secondary_window_minutes ELSE 0 END",
		}
	}
	return scopedQuotaSource{EventScope: scope, EventFilter: "1=1", UsedExpr: "used_percent", ResetExpr: "reset_at", WindowExpr: "window_minutes"}
}

type scopedQuotaObservation struct {
	ID            int64
	RequestedAt   int64
	ObservedAt    int64
	UsedPercent   float64
	ResetAt       int64
	WindowMinutes int64
	PlanType      string
}

type scopedCycleDefinition struct {
	Cycle            quotaCycle
	ObservationStart int
	ObservationEnd   int
}

func (s *store) latestQuotaScopeSeries(ctx context.Context, account, scope string, limit int) (scopedQuotaSeries, error) {
	return s.latestQuotaScopeSeriesAt(ctx, account, scope, limit, time.Now().Unix())
}

func (s *store) latestQuotaScopeSeriesAt(ctx context.Context, account, scope string, limit int, now int64) (scopedQuotaSeries, error) {
	definitions, err := s.quotaScopeCycleDefinitions(ctx, account, scope)
	if err != nil || len(definitions) == 0 {
		return scopedQuotaSeries{Scope: scope, Points: []scopedQuotaPoint{}}, err
	}
	series, err := s.quotaScopeSeriesForCycle(ctx, account, scope, definitions[0], limit)
	if err != nil {
		return series, err
	}
	return series, nil
}

func (s *store) quotaScopeSeries(ctx context.Context, account, scope string, selectedResetAt int64, limit int) (scopedQuotaSeries, error) {
	definitions, err := s.quotaScopeCycleDefinitions(ctx, account, scope)
	if err != nil {
		return scopedQuotaSeries{Scope: scope, Points: []scopedQuotaPoint{}}, err
	}
	for _, definition := range definitions {
		if selectedResetAt == 0 || definition.Cycle.ResetAt == selectedResetAt {
			return s.quotaScopeSeriesForCycle(ctx, account, scope, definition, limit)
		}
	}
	return scopedQuotaSeries{Scope: scope, Points: []scopedQuotaPoint{}}, nil
}

func (s *store) quotaScopeSeriesForCycle(ctx context.Context, account, scope string, definition scopedCycleDefinition, limit int) (scopedQuotaSeries, error) {
	cycle := definition.Cycle
	series := scopedQuotaSeries{
		Scope:            scope,
		StartedAt:        cycle.StartedAt,
		ResetAt:          cycle.ResetAt,
		WindowMinutes:    cycle.WindowMinutes,
		PlanType:         cycle.PlanType,
		LastObservedAt:   cycle.LastSampleAt,
		ScheduleInferred: cycle.ScheduleInferred,
		Points:           []scopedQuotaPoint{},
	}
	if account == "" || scope == "" || cycle.ID == 0 {
		return series, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	endAt := scopedCycleEnd(cycle)
	if endAt <= cycle.StartedAt {
		return series, nil
	}
	source := quotaScopeSource(scope)
	pointTime := `CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END`
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(`+source.UsedExpr+`),0),COUNT(`+source.UsedExpr+`)
FROM quota_history
WHERE account=? AND quota_scope=? AND `+source.EventFilter+` AND failed=0 AND `+source.UsedExpr+` IS NOT NULL
	AND `+pointTime+`>=? AND `+pointTime+`<?`, account, source.EventScope, cycle.StartedAt, endAt).
		Scan(&series.UsedPercent, &series.ObservationCount); err != nil {
		return series, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH scoped AS (
	SELECT id,requested_at,CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END AS point_time,
		`+source.UsedExpr+` AS used_percent,failed,
		SUM(total_tokens) OVER (ORDER BY requested_at,id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS window_tokens,
		SUM(cost_usd) OVER (ORDER BY requested_at,id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS window_cost_usd,
		COUNT(*) OVER (ORDER BY requested_at,id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS requests
	FROM quota_history
	WHERE account=? AND quota_scope=? AND `+source.EventFilter+` AND requested_at>=? AND requested_at<?
), quota_points AS (
	SELECT *,
		MAX(used_percent) OVER (ORDER BY point_time,id ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING) AS prior_peak,
		ROW_NUMBER() OVER (ORDER BY point_time DESC,id DESC) AS newest
	FROM scoped
	WHERE failed=0 AND used_percent IS NOT NULL AND point_time>=? AND point_time<?
)
SELECT point_time,used_percent,window_tokens,window_cost_usd,requests
FROM quota_points
WHERE prior_peak IS NULL OR used_percent>prior_peak OR newest=1
ORDER BY point_time,id
LIMIT ?`, account, source.EventScope, cycle.StartedAt, endAt, cycle.StartedAt, endAt, limit)
	if err != nil {
		return series, err
	}
	defer rows.Close()
	for rows.Next() {
		var point scopedQuotaPoint
		if err = rows.Scan(&point.Time, &point.UsedPercent, &point.WindowTokens, &point.WindowCostUSD, &point.Requests); err != nil {
			return series, err
		}
		point.ResetAt = cycle.ResetAt
		point.WindowMinutes = cycle.WindowMinutes
		point.PlanType = cycle.PlanType
		series.Points = append(series.Points, point)
	}
	if err = rows.Err(); err != nil {
		return series, err
	}
	return series, nil
}

func (s *store) quotaScopeCycles(ctx context.Context, account, scope string) ([]quotaCycle, error) {
	definitions, err := s.quotaScopeCycleDefinitions(ctx, account, scope)
	if err != nil {
		return nil, err
	}
	cycles := make([]quotaCycle, 0, len(definitions))
	for _, definition := range definitions {
		cycles = append(cycles, definition.Cycle)
	}
	return cycles, nil
}

func (s *store) quotaScopeCycleDefinitions(ctx context.Context, account, scope string) ([]scopedCycleDefinition, error) {
	observations, err := s.quotaScopeObservations(ctx, account, scope)
	if err != nil || len(observations) == 0 {
		return nil, err
	}
	startIndex := 0
	cycleStart := scopedDeclaredStart(observations[0], observations[0].ObservedAt)
	cycleReset := observations[0].ResetAt
	cycleWindow := observations[0].WindowMinutes
	cyclePlan := observations[0].PlanType
	fixedStart := false
	peak := observations[0].UsedPercent
	pendingSchedule := -1
	pendingLow := -1
	pendingLowCount := 0
	pendingLowLast := float64(0)
	definitions := make([]scopedCycleDefinition, 0, 8)

	appendCycle := func(endIndex int, endedAt int64, reason string) {
		if endIndex <= startIndex {
			return
		}
		definition := buildScopedCycleDefinition(observations, startIndex, endIndex, cycleStart, cycleReset, cycleWindow, cyclePlan, endedAt, reason)
		definitions = append(definitions, definition)
	}
	recomputePeak := func(from, through int) float64 {
		value := float64(0)
		for index := from; index <= through && index < len(observations); index++ {
			if observations[index].UsedPercent > value {
				value = observations[index].UsedPercent
			}
		}
		return value
	}

	for index := 1; index < len(observations); index++ {
		observation := observations[index]
		if observation.ResetAt != cycleReset {
			if scopedScheduledTransitionCandidate(cycleReset, observation) &&
				(cycleWindow == 0 || cycleWindow == observation.WindowMinutes) && compatiblePlan(cyclePlan, observation.PlanType) {
				if pendingSchedule >= 0 && index == pendingSchedule+1 && sameScopedQuotaRegime(observations[pendingSchedule], observation) {
					oldReset := cycleReset
					appendCycle(pendingSchedule, oldReset, "scheduled_reset")
					startIndex = pendingSchedule
					cycleStart = scopedDeclaredStart(observations[pendingSchedule], oldReset)
					if cycleStart < oldReset {
						cycleStart = oldReset
					}
					cycleReset = observation.ResetAt
					cycleWindow = observation.WindowMinutes
					cyclePlan = observation.PlanType
					fixedStart = true
					peak = recomputePeak(startIndex, index)
					pendingSchedule = -1
					pendingLow = -1
					pendingLowCount = 0
					continue
				}
				pendingSchedule = index
				continue
			}
			// A reset timestamp that changes before the old boundary while usage
			// continues is a schedule correction, not an observed reset.
			cycleReset = observation.ResetAt
			cycleWindow = observation.WindowMinutes
			if observation.PlanType != "" {
				cyclePlan = observation.PlanType
			}
			if !fixedStart {
				cycleStart = scopedDeclaredStart(observation, cycleStart)
			}
			pendingSchedule = -1
		} else {
			pendingSchedule = -1
			if observation.WindowMinutes > 0 {
				cycleWindow = observation.WindowMinutes
			}
			if observation.PlanType != "" {
				cyclePlan = observation.PlanType
			}
		}

		if resetCandidate(peak, observation.UsedPercent) {
			if pendingLow < 0 || !sameScopedQuotaRegime(observations[pendingLow], observation) || observation.UsedPercent+resetPercentTolerance < pendingLowLast {
				pendingLow = index
				pendingLowCount = 1
			} else {
				pendingLowCount++
			}
			pendingLowLast = observation.UsedPercent
			if pendingLowCount >= resetConfirmationSamples && observation.ObservedAt-observations[pendingLow].ObservedAt >= resetConfirmationMinSeconds {
				boundary := observations[pendingLow].ObservedAt
				appendCycle(pendingLow, boundary, "early_reset")
				startIndex = pendingLow
				cycleStart = boundary
				cycleReset = observation.ResetAt
				cycleWindow = observation.WindowMinutes
				cyclePlan = observation.PlanType
				fixedStart = true
				peak = recomputePeak(startIndex, index)
				pendingLow = -1
				pendingLowCount = 0
			}
			continue
		}
		pendingLow = -1
		pendingLowCount = 0
		if observation.UsedPercent > peak {
			peak = observation.UsedPercent
		}
	}
	appendCycle(len(observations), 0, "")
	if len(definitions) == 0 {
		return nil, nil
	}
	definitions[len(definitions)-1].Cycle.Current = true
	for left, right := 0, len(definitions)-1; left < right; left, right = left+1, right-1 {
		definitions[left], definitions[right] = definitions[right], definitions[left]
	}
	return definitions, nil
}

func (s *store) quotaScopeObservations(ctx context.Context, account, scope string) ([]scopedQuotaObservation, error) {
	source := quotaScopeSource(scope)
	rows, err := s.db.QueryContext(ctx, `SELECT id,requested_at,
	CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END,
	`+source.UsedExpr+`,`+source.ResetExpr+`,`+source.WindowExpr+`,plan_type
FROM quota_history
WHERE account=? AND quota_scope=? AND `+source.EventFilter+` AND failed=0
	AND `+source.UsedExpr+` IS NOT NULL AND `+source.ResetExpr+`>0 AND `+source.WindowExpr+`>0
ORDER BY CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END,id`, account, source.EventScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	observations := make([]scopedQuotaObservation, 0, 256)
	for rows.Next() {
		var observation scopedQuotaObservation
		if err = rows.Scan(&observation.ID, &observation.RequestedAt, &observation.ObservedAt, &observation.UsedPercent, &observation.ResetAt, &observation.WindowMinutes, &observation.PlanType); err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func buildScopedCycleDefinition(observations []scopedQuotaObservation, startIndex, endIndex int, startedAt, resetAt, windowMinutes int64, planType string, endedAt int64, reason string) scopedCycleDefinition {
	first := startIndex
	for first < endIndex && observations[first].ObservedAt < startedAt {
		first++
	}
	if first >= endIndex {
		first = startIndex
	}
	last := endIndex - 1
	cycle := quotaCycle{
		ID:               observations[first].ID,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		ResetAt:          resetAt,
		WindowMinutes:    windowMinutes,
		PlanType:         planType,
		CloseReason:      reason,
		FirstSampleAt:    observations[first].ObservedAt,
		LastSampleAt:     observations[last].ObservedAt,
		StartPercent:     observations[first].UsedPercent,
		EndPercent:       observations[last].UsedPercent,
		PeakPercent:      observations[first].UsedPercent,
		ObservedComplete: observations[first].UsedPercent <= resetLowPercent,
	}
	for index := first + 1; index < endIndex; index++ {
		if endedAt > 0 && observations[index].ObservedAt >= endedAt {
			break
		}
		cycle.EndPercent = observations[index].UsedPercent
		cycle.LastSampleAt = observations[index].ObservedAt
		if observations[index].UsedPercent > cycle.PeakPercent {
			cycle.PeakPercent = observations[index].UsedPercent
		}
	}
	return scopedCycleDefinition{Cycle: cycle, ObservationStart: startIndex, ObservationEnd: endIndex}
}

func scopedDeclaredStart(observation scopedQuotaObservation, fallback int64) int64 {
	startedAt := observation.ResetAt - observation.WindowMinutes*60
	if startedAt <= 0 || startedAt > observation.ObservedAt+60 {
		return fallback
	}
	return startedAt
}

func scopedScheduledTransitionCandidate(currentResetAt int64, observation scopedQuotaObservation) bool {
	if currentResetAt <= 0 || observation.ResetAt <= 0 || observation.WindowMinutes <= 0 {
		return false
	}
	declaredStart := observation.ResetAt - observation.WindowMinutes*60
	return observation.ResetAt > currentResetAt &&
		declaredStart >= currentResetAt-scheduledResetTolerance &&
		declaredStart <= observation.ObservedAt+scheduledResetTolerance &&
		observation.ObservedAt >= currentResetAt-scheduledResetTolerance
}

func sameScopedQuotaRegime(left, right scopedQuotaObservation) bool {
	if left.ResetAt != right.ResetAt || left.WindowMinutes != right.WindowMinutes {
		return false
	}
	return left.PlanType == "" || right.PlanType == "" || left.PlanType == right.PlanType
}

func scopedCycleEnd(cycle quotaCycle) int64 {
	if cycle.EndedAt > cycle.StartedAt {
		return cycle.EndedAt
	}
	if cycle.ResetAt > cycle.StartedAt {
		return cycle.ResetAt
	}
	return cycle.LastSampleAt + 1
}

func (s *store) quotaScopeCycleGrowth(ctx context.Context, account, scope string, cycle quotaCycle, startAt, endAt int64) (float64, bool, error) {
	return s.scopedCycleGrowth(ctx, account, scope, cycle, startAt, endAt, true)
}

func (s *store) scopedCycleGrowth(ctx context.Context, account, scope string, cycle quotaCycle, startAt, endAt int64, resetBaseline bool) (float64, bool, error) {
	source := quotaScopeSource(scope)
	pointTime := `CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END`
	cycleEnd := scopedCycleEnd(cycle)
	var endPeak sql.NullFloat64
	query := `SELECT MAX(` + source.UsedExpr + `) FROM quota_history
WHERE account=? AND quota_scope=? AND ` + source.EventFilter + ` AND failed=0 AND ` + source.UsedExpr + ` IS NOT NULL
	AND ` + pointTime + `>=? AND ` + pointTime + `<? AND ` + pointTime + `<?`
	if err := s.db.QueryRowContext(ctx, query, account, source.EventScope, cycle.StartedAt, endAt, cycleEnd).Scan(&endPeak); err != nil {
		return 0, false, err
	}
	if !endPeak.Valid {
		return 0, false, nil
	}
	if resetBaseline && cycle.StartedAt >= startAt {
		return maxFloat(0, endPeak.Float64), true, nil
	}
	var baseline sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, query, account, source.EventScope, cycle.StartedAt, startAt, cycleEnd).Scan(&baseline); err != nil {
		return 0, false, err
	}
	if baseline.Valid {
		return maxFloat(0, endPeak.Float64-baseline.Float64), true, nil
	}
	var firstInMonth sql.NullFloat64
	firstQuery := `SELECT ` + source.UsedExpr + ` FROM quota_history
WHERE account=? AND quota_scope=? AND ` + source.EventFilter + ` AND failed=0 AND ` + source.UsedExpr + ` IS NOT NULL
	AND ` + pointTime + `>=? AND ` + pointTime + `<? AND ` + pointTime + `<? ORDER BY ` + pointTime + `,id LIMIT 1`
	if err := s.db.QueryRowContext(ctx, firstQuery, account, source.EventScope, max(startAt, cycle.StartedAt), endAt, cycleEnd).Scan(&firstInMonth); err != nil && err != sql.ErrNoRows {
		return 0, false, err
	}
	if firstInMonth.Valid {
		return maxFloat(0, endPeak.Float64-firstInMonth.Float64), false, nil
	}
	return 0, false, nil
}

func (s *store) hasFiveHourWeeklyQuota(ctx context.Context, account string) (bool, error) {
	return s.hasFiveHourWeeklyQuotaForScope(ctx, account, mainQuotaScope)
}

func (s *store) hasSparkFiveHourWeeklyQuota(ctx context.Context, account string) (bool, error) {
	return s.hasFiveHourWeeklyQuotaForScope(ctx, account, sparkQuotaScope)
}

func (s *store) hasFiveHourWeeklyQuotaForScope(ctx context.Context, account, eventScope string) (bool, error) {
	var primaryWindow, secondaryReset, secondaryWindow int64
	var secondaryUsed sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `SELECT window_minutes,secondary_used_percent,secondary_reset_at,secondary_window_minutes
FROM quota_history
WHERE account=? AND quota_scope=? AND failed=0 AND used_percent IS NOT NULL AND reset_at>0 AND window_minutes>0
ORDER BY CASE WHEN observed_at>0 THEN observed_at ELSE requested_at END DESC,id DESC
LIMIT 1`, account, eventScope).Scan(&primaryWindow, &secondaryUsed, &secondaryReset, &secondaryWindow)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isFiveHourWindow(primaryWindow) && secondaryUsed.Valid && secondaryReset > 0 && isWeeklyWindow(secondaryWindow), nil
}

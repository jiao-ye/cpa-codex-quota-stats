package main

import (
	"math"
	"testing"
)

func TestReviewerOfficialFastCreditBreakdownMatchesPublishedValuation(t *testing.T) {
	d := usageDetail{InputTokens: 100000, CacheReadTokens: 20000, OutputTokens: 10000}
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol", "gpt-6-astra", "gpt-5.6-sol"} {
		standard, ok := officialCreditsForUsage(model, "standard", d.InputTokens, d.CacheReadTokens, 0, d.OutputTokens)
		fast, fastOK := officialCreditsForUsage(model, "fast", d.InputTokens, d.CacheReadTokens, 0, d.OutputTokens)
		if !ok || !fastOK || math.Abs(fast-2*standard) > 1e-9 {
			t.Errorf("%s paid credits Fast=%g known=%v; want 2x Standard=%g", model, fast, fastOK, standard)
		}
		p := price{Model: model}
		cfg := defaultConfig()
		cfg.PricingMode = pricingModeCredits
		if got := calculateCost(p, d, "fast", cfg); math.Abs(got-fast) > 1e-9 {
			t.Errorf("%s price=%g breakdown=%g", model, got, fast)
		}
	}
}

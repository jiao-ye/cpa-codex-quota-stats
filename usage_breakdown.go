package main

import (
	"strings"
)

func officialCreditsForUsage(model, tier string, input, cached, written, output int64) (float64, bool) {
	tier = strings.ToLower(strings.TrimSpace(tier))
	p, ok := officialCodexCreditPrice(model)
	if !ok {
		return 0, false
	}
	multiplier := 1.0
	if isFastTier(tier) {
		switch normalizeModel(model) {
		case "gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna", "gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5":
			multiplier = 2
		case "gpt-5.4":
			multiplier = 2
		default:
			return 0, false
		}
	} else if tier != "" && tier != "auto" && tier != "default" && tier != "standard" {
		return 0, false
	}
	uncached := input - cached - written
	if uncached < 0 {
		uncached = 0
	}
	// Codex has no separate credit charge for cache writes.
	return (float64(uncached)*p.Input + float64(cached)*p.CacheRead + float64(output)*p.Output) * multiplier / 1_000_000, true
}

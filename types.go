package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

const (
	pluginID   = "cpa-quota-estimator"
	pluginName = "Codex Quota Statistics"
)

var pluginVersion = "0.21.4"

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type usageRecord struct {
	Provider        string       `json:"Provider"`
	ExecutorType    string       `json:"ExecutorType"`
	Model           string       `json:"Model"`
	Alias           string       `json:"Alias"`
	APIKey          string       `json:"APIKey"`
	AuthID          string       `json:"AuthID"`
	AuthIndex       string       `json:"AuthIndex"`
	AuthType        string       `json:"AuthType"`
	Source          string       `json:"Source"`
	ReasoningEffort string       `json:"ReasoningEffort"`
	ServiceTier     string       `json:"ServiceTier"`
	Generate        bool         `json:"Generate"`
	RequestedAt     time.Time    `json:"RequestedAt"`
	Latency         int64        `json:"Latency"`
	TTFT            int64        `json:"TTFT"`
	Failed          bool         `json:"Failed"`
	Failure         usageFailure `json:"Failure"`
	Detail          usageDetail  `json:"Detail"`
	ResponseHeaders http.Header  `json:"ResponseHeaders"`
}

type usageFailure struct {
	StatusCode int    `json:"StatusCode"`
	Body       string `json:"Body"`
}

type usageDetail struct {
	InputTokens         int64 `json:"InputTokens"`
	OutputTokens        int64 `json:"OutputTokens"`
	ReasoningTokens     int64 `json:"ReasoningTokens"`
	CachedTokens        int64 `json:"CachedTokens"`
	CacheReadTokens     int64 `json:"CacheReadTokens"`
	CacheCreationTokens int64 `json:"CacheCreationTokens"`
	TotalTokens         int64 `json:"TotalTokens"`
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type managementRequest struct {
	Method         string      `json:"Method"`
	Path           string      `json:"Path"`
	Headers        http.Header `json:"Headers"`
	Query          url.Values  `json:"Query"`
	Body           []byte      `json:"Body"`
	HostCallbackID string      `json:"host_callback_id,omitempty"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers,omitempty"`
	Body       []byte      `json:"Body,omitempty"`
}

type config struct {
	Enabled                  bool             `yaml:"enabled"`
	DataPath                 string           `yaml:"data_path"`
	SampleIntervalMinutes    int              `yaml:"sample_interval_minutes"`
	PriceSourceURL           string           `yaml:"price_source_url"`
	PriceSyncIntervalMinutes int              `yaml:"price_sync_interval_minutes"`
	PricingMode              string           `yaml:"pricing_mode"`
	PriceCatalog             map[string]price `yaml:"-"`
	LongContextThreshold     int64            `yaml:"long_context_threshold"`
	HistoryDays              int              `yaml:"history_days"`
	CaptureCodexHeaders      bool             `yaml:"capture_codex_headers"`
}

func defaultConfig() config {
	return config{
		Enabled:                  true,
		DataPath:                 "/CLIProxyAPI/data/cpa-quota-estimator.sqlite",
		SampleIntervalMinutes:    5,
		PriceSourceURL:           "https://models.dev/catalog.json",
		PriceSyncIntervalMinutes: 1440,
		PricingMode:              pricingModeCredits,
		LongContextThreshold:     272000,
		HistoryDays:              365,
	}
}

type price struct {
	Model      string  `json:"model"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	LongInput  float64 `json:"long_input"`
	LongOutput float64 `json:"long_output"`
	LongRead   float64 `json:"long_cache_read"`
	LongWrite  float64 `json:"long_cache_write"`
	FastInput  float64 `json:"fast_input"`
	FastOutput float64 `json:"fast_output"`
	FastRead   float64 `json:"fast_cache_read"`
	FastWrite  float64 `json:"fast_cache_write"`
	Source     string  `json:"source"`
	UpdatedAt  int64   `json:"updated_at"`
}

type event struct {
	IngestID               string
	RequestedAt            int64
	ObservedAt             int64
	Account                string
	Provider               string
	Model                  string
	Alias                  string
	ServiceTier            string
	InputTokens            int64
	OutputTokens           int64
	ReasoningTokens        int64
	CacheReadTokens        int64
	CacheWriteTokens       int64
	TotalTokens            int64
	CostUSD                float64
	Failed                 bool
	StatusCode             int
	UsedPercent            *float64
	ResetAt                int64
	WindowMinutes          int64
	SecondaryUsedPercent   *float64
	SecondaryResetAt       int64
	SecondaryWindowMinutes int64
	PlanType               string
	QuotaScope             string
	CodexHeadersJSON       string
}

type quotaPoint struct {
	CycleID       int64   `json:"cycle_id"`
	CycleStart    int64   `json:"cycle_start"`
	Time          int64   `json:"time"`
	UsedPercent   float64 `json:"used_percent"`
	ResetAt       int64   `json:"reset_at"`
	WindowMinutes int64   `json:"window_minutes"`
	WindowTokens  int64   `json:"window_tokens"`
	WindowCostUSD float64 `json:"window_cost_usd"`
	Requests      int64   `json:"requests"`
	Anomalous     bool    `json:"anomalous,omitempty"`
	BreakBefore   bool    `json:"break_before,omitempty"`
}

type quotaRegimeAnomaly struct {
	CycleID              int64   `json:"cycle_id"`
	Kind                 string  `json:"kind"`
	BeforeAt             int64   `json:"before_at"`
	StartedAt            int64   `json:"started_at"`
	PeakAt               int64   `json:"peak_at"`
	EndedAt              int64   `json:"ended_at"`
	BeforeUsedPercent    float64 `json:"before_used_percent"`
	AnomalousUsedPercent float64 `json:"anomalous_used_percent"`
	PeakUsedPercent      float64 `json:"peak_used_percent"`
	RestoredUsedPercent  float64 `json:"restored_used_percent"`
	BeforeResetAt        int64   `json:"before_reset_at"`
	AnomalousResetAt     int64   `json:"anomalous_reset_at"`
	RestoredResetAt      int64   `json:"restored_reset_at"`
	ObservationCount     int64   `json:"observation_count"`
}

type scopedQuotaPoint struct {
	Time          int64   `json:"time"`
	UsedPercent   float64 `json:"used_percent"`
	ResetAt       int64   `json:"reset_at"`
	WindowMinutes int64   `json:"window_minutes"`
	PlanType      string  `json:"plan_type"`
	WindowTokens  int64   `json:"window_tokens"`
	WindowCostUSD float64 `json:"window_cost_usd"`
	Requests      int64   `json:"requests"`
}

type scopedQuotaSeries struct {
	Scope            string             `json:"scope"`
	StartedAt        int64              `json:"started_at"`
	ResetAt          int64              `json:"reset_at"`
	WindowMinutes    int64              `json:"window_minutes"`
	PlanType         string             `json:"plan_type"`
	UsedPercent      float64            `json:"used_percent"`
	ObservationCount int64              `json:"observation_count"`
	LastObservedAt   int64              `json:"last_observed_at,omitempty"`
	ScheduleInferred bool               `json:"schedule_inferred,omitempty"`
	Points           []scopedQuotaPoint `json:"points"`
}

type quotaCycle struct {
	ID               int64   `json:"id"`
	StartedAt        int64   `json:"started_at"`
	EndedAt          int64   `json:"ended_at,omitempty"`
	ResetAt          int64   `json:"reset_at"`
	WindowMinutes    int64   `json:"window_minutes"`
	PlanType         string  `json:"plan_type"`
	CloseReason      string  `json:"close_reason,omitempty"`
	FirstSampleAt    int64   `json:"first_sample_at"`
	LastSampleAt     int64   `json:"last_sample_at"`
	StartPercent     float64 `json:"start_percent"`
	EndPercent       float64 `json:"end_percent"`
	PeakPercent      float64 `json:"peak_percent"`
	ActualTokens     int64   `json:"actual_tokens"`
	ActualCostUSD    float64 `json:"actual_cost_usd"`
	Requests         int64   `json:"requests"`
	Current          bool    `json:"current"`
	ObservedComplete bool    `json:"observed_complete"`
	ScheduleInferred bool    `json:"schedule_inferred,omitempty"`
}

type quotaWindow struct {
	ResetAt       int64   `json:"reset_at"`
	WindowStart   int64   `json:"window_start"`
	WindowMinutes int64   `json:"window_minutes"`
	PlanType      string  `json:"plan_type"`
	FirstSampleAt int64   `json:"first_sample_at"`
	LastSampleAt  int64   `json:"last_sample_at"`
	StartPercent  float64 `json:"start_percent"`
	EndPercent    float64 `json:"end_percent"`
	WindowTokens  int64   `json:"window_tokens"`
	WindowCostUSD float64 `json:"window_cost_usd"`
	Requests      int64   `json:"requests"`
}

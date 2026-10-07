package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

//go:embed web/dashboard.html
var dashboardTemplate []byte

//go:embed web/accounting.js
var accountingScript []byte

//go:embed web/accounting.css
var accountingStyles []byte

//go:embed web/stats.js
var statsScript []byte

//go:embed web/stats.css
var statsStyles []byte

var dashboardHTML = func() []byte {
	html := bytes.Replace(dashboardTemplate, []byte("/*__STATS_STYLE__*/"), append(statsStyles, accountingStyles...), 1)
	return bytes.Replace(html, []byte("//__STATS_SCRIPT__"), append(append(statsScript, '\n'), accountingScript...), 1)
}()

func (a *app) handleManagement(req managementRequest) managementResponse {
	if req.Path == "/dashboard" || req.Path == "/"+pluginID+"/dashboard" ||
		req.Path == "/v0/resource/plugins/"+pluginID+"/dashboard" {
		if req.Method != "GET" {
			return textResponse(405, "method not allowed")
		}
		return managementResponse{StatusCode: 200, Headers: map[string][]string{
			"Content-Type": {"text/html; charset=utf-8"}, "Cache-Control": {"no-store"},
		}, Body: dashboardHTML}
	}
	path := strings.TrimPrefix(strings.TrimPrefix(req.Path, "/v0/management"), "/"+pluginID)
	if path != "/overview" && path != "/usage" && path != "/accounting" && path != "/subscriptions" {
		return textResponse(404, "not found")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store == nil {
		return textResponse(503, "plugin store is not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if path == "/subscriptions" {
		if req.Method != "POST" && req.Method != "DELETE" {
			return textResponse(405, "method not allowed")
		}
		var payment subscriptionPayment
		if err := json.Unmarshal(req.Body, &payment); err != nil {
			return textResponse(400, "invalid payment")
		}
		var err error
		if req.Method == "POST" {
			err = a.store.savePayment(ctx, payment)
		} else {
			err = a.store.deletePayment(ctx, payment.Account, payment.ID)
		}
		if err != nil {
			return textResponse(400, err.Error())
		}
		return jsonResponse(200, map[string]bool{"ok": true})
	}
	if req.Method != "GET" {
		return textResponse(405, "method not allowed")
	}
	if path == "/overview" {
		accounts, err := a.store.accounts(ctx)
		if err != nil {
			return textResponse(500, err.Error())
		}
		if accounts == nil {
			accounts = []string{}
		}
		return jsonResponse(200, map[string]any{"plugin_version": pluginVersion, "accounts": accounts,
			"dropped_usage_events": a.totalDroppedUsage(ctx, a.store)})
	}
	account := req.Query.Get("account")
	if account == "" {
		return textResponse(400, "account is required")
	}
	if path == "/accounting" {
		limit, _ := strconv.Atoi(req.Query.Get("limit"))
		offset, _ := strconv.Atoi(req.Query.Get("offset"))
		result, err := a.store.accounting(ctx, account, limit, offset)
		if err != nil {
			return textResponse(500, err.Error())
		}
		result.Dropped = a.totalDroppedUsage(ctx, a.store)
		return jsonResponse(200, result)
	}
	days := 7
	if raw := req.Query.Get("days"); raw != "" {
		var err error
		days, err = strconv.Atoi(raw)
		if err != nil || days != 1 && days != 7 && days != 30 {
			return textResponse(400, "days must be 1, 7, or 30")
		}
	}
	end := time.Now().Unix() + 1
	result, err := a.store.actualUsageBreakdown(ctx, account, end-int64(days)*86400, end)
	if err != nil {
		return textResponse(500, err.Error())
	}
	return jsonResponse(200, result)
}

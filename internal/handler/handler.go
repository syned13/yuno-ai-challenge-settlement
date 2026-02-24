package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/denys-rosario/settlement-reconciler/internal/generator"
	"github.com/denys-rosario/settlement-reconciler/internal/models"
	"github.com/denys-rosario/settlement-reconciler/internal/reconciler"
	"github.com/denys-rosario/settlement-reconciler/internal/store"
)

// Handler holds dependencies for HTTP request handling.
type Handler struct {
	store      *store.Store
	reconciler *reconciler.Reconciler
	config     models.ReconciliationConfig
	runSeq     int
}

func New(s *store.Store, r *reconciler.Reconciler, cfg models.ReconciliationConfig) *Handler {
	return &Handler{store: s, reconciler: r, config: cfg}
}

// RegisterRoutes wires all endpoints onto the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /docs", h.docs)
	mux.HandleFunc("GET /health", h.health)

	// Data ingestion
	mux.HandleFunc("POST /api/v1/transactions", h.uploadTransactions)
	mux.HandleFunc("POST /api/v1/settlements", h.uploadSettlements)

	// Reconciliation
	mux.HandleFunc("POST /api/v1/reconciliation/run", h.triggerReconciliation)
	mux.HandleFunc("GET /api/v1/reconciliation/runs", h.listRuns)
	mux.HandleFunc("GET /api/v1/reconciliation/runs/{runID}", h.getRun)
	mux.HandleFunc("GET /api/v1/reconciliation/runs/{runID}/report", h.getReport)

	// Query
	mux.HandleFunc("GET /api/v1/transactions/{txnID}/reconciliation", h.getTransactionReconciliation)

	// Configuration
	mux.HandleFunc("GET /api/v1/config", h.getConfig)
	mux.HandleFunc("PUT /api/v1/config", h.updateConfig)

	// Test data
	mux.HandleFunc("POST /api/v1/test-data/generate", h.generateTestData)
}

// --- Index ---

func (h *Handler) index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":     "AuraCommerce Settlement Reconciliation Service",
		"version":     "1.0.0",
		"status":      "running",
		"docs":        "/docs",
		"health":      "/health",
		"api_base":    "/api/v1",
		"endpoints": map[string]string{
			"generate_test_data":    "POST /api/v1/test-data/generate",
			"upload_transactions":   "POST /api/v1/transactions",
			"upload_settlements":    "POST /api/v1/settlements",
			"run_reconciliation":    "POST /api/v1/reconciliation/run",
			"list_runs":             "GET  /api/v1/reconciliation/runs",
			"get_run":               "GET  /api/v1/reconciliation/runs/{runID}",
			"get_report":            "GET  /api/v1/reconciliation/runs/{runID}/report",
			"query_transaction":     "GET  /api/v1/transactions/{txnID}/reconciliation",
			"get_config":            "GET  /api/v1/config",
			"update_config":         "PUT  /api/v1/config",
		},
	})
}

// --- Docs ---

func (h *Handler) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(docsHTML))
}

const docsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Settlement Reconciliation API — Docs</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0a0a0f; color: #e0e0e0; line-height: 1.6; padding: 2rem; max-width: 960px; margin: 0 auto; }
  h1 { color: #fff; font-size: 1.8rem; margin-bottom: 0.5rem; }
  h2 { color: #fff; font-size: 1.3rem; margin-top: 2rem; margin-bottom: 0.75rem; border-bottom: 1px solid rgba(255,255,255,0.08); padding-bottom: 0.5rem; }
  h3 { color: #a0d4ff; font-size: 1rem; margin-top: 1.25rem; margin-bottom: 0.5rem; }
  p { color: rgba(255,255,255,0.6); margin-bottom: 0.75rem; }
  .subtitle { color: rgba(255,255,255,0.4); font-size: 0.95rem; margin-bottom: 2rem; }
  .badge { display: inline-block; padding: 2px 10px; border-radius: 9999px; font-size: 0.7rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; }
  .badge-get { background: rgba(34,197,94,0.15); color: #22c55e; }
  .badge-post { background: rgba(59,130,246,0.15); color: #3b82f6; }
  .badge-put { background: rgba(234,179,8,0.15); color: #eab308; }
  .endpoint { background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06); border-radius: 8px; padding: 1rem 1.25rem; margin-bottom: 0.75rem; }
  .endpoint-header { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.25rem; }
  .endpoint-path { font-family: "SF Mono", "Fira Code", monospace; color: #fff; font-size: 0.9rem; }
  .endpoint-desc { color: rgba(255,255,255,0.5); font-size: 0.85rem; }
  pre { background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08); border-radius: 6px; padding: 1rem; overflow-x: auto; margin: 0.5rem 0 1rem; font-size: 0.82rem; line-height: 1.5; }
  code { font-family: "SF Mono", "Fira Code", monospace; color: #a0d4ff; font-size: 0.85rem; }
  pre code { color: #d4d4d4; }
  table { width: 100%; border-collapse: collapse; margin: 0.75rem 0; font-size: 0.85rem; }
  th, td { text-align: left; padding: 0.5rem 0.75rem; border-bottom: 1px solid rgba(255,255,255,0.06); }
  th { color: rgba(255,255,255,0.5); font-weight: 600; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; }
  td code { background: rgba(160,212,255,0.1); padding: 1px 6px; border-radius: 3px; }
  .status-table td:first-child { font-weight: 600; color: #fff; }
  a { color: #a0d4ff; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .try-it { margin-top: 0.5rem; }
  .try-it summary { cursor: pointer; color: rgba(255,255,255,0.5); font-size: 0.8rem; }
  .try-it summary:hover { color: #a0d4ff; }
</style>
</head>
<body>

<h1>Settlement Reconciliation API</h1>
<p class="subtitle">AuraCommerce — Automated settlement matching and discrepancy detection</p>

<h2>Quick Start</h2>
<p>The fastest way to see the service in action:</p>
<pre><code># 1. Generate test data (200 transactions + 200 settlements)
curl -X POST /api/v1/test-data/generate

# 2. Run reconciliation
curl -X POST /api/v1/reconciliation/run

# 3. View the full report
curl /api/v1/reconciliation/runs/RUN-0001/report</code></pre>
<p>If the server was started with <code>--seed-data</code>, data is already loaded and a reconciliation run (<code>SEED-0001</code>) is available.</p>

<h2>Endpoints</h2>

<h3>System</h3>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/</span>
  </div>
  <p class="endpoint-desc">Service info and endpoint listing (JSON)</p>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/health</span>
  </div>
  <p class="endpoint-desc">Health check</p>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/docs</span>
  </div>
  <p class="endpoint-desc">This documentation page</p>
</div>

<h3>Data Ingestion</h3>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-post">POST</span>
    <span class="endpoint-path">/api/v1/transactions</span>
  </div>
  <p class="endpoint-desc">Upload internal transaction records (JSON array)</p>
  <details class="try-it"><summary>Example</summary>
  <pre><code>curl -X POST /api/v1/transactions \
  -H "Content-Type: application/json" \
  -d '[{
    "id": "TXN-001", "order_id": "ORD-001",
    "processor_name": "PaySureMX", "processor_txn_id": "PSM-001",
    "amount": 100.00, "currency": "MXN", "country": "MX",
    "status": "captured",
    "authorized_at": "2025-01-15T10:00:00Z",
    "customer_email": "test@example.com",
    "payment_method": "credit_card"
  }]'</code></pre>
  </details>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-post">POST</span>
    <span class="endpoint-path">/api/v1/settlements</span>
  </div>
  <p class="endpoint-desc">Upload processor settlement records (JSON array)</p>
  <details class="try-it"><summary>Example</summary>
  <pre><code>curl -X POST /api/v1/settlements \
  -H "Content-Type: application/json" \
  -d '[{
    "id": "STL-001", "processor_name": "PaySureMX",
    "processor_txn_id": "PSM-001", "order_reference": "ORD-001",
    "gross_amount": 100.00, "fee_amount": 2.50, "net_amount": 97.50,
    "currency": "MXN",
    "settled_at": "2025-01-17T14:00:00Z",
    "settlement_batch_id": "BATCH-20250117"
  }]'</code></pre>
  </details>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-post">POST</span>
    <span class="endpoint-path">/api/v1/test-data/generate</span>
  </div>
  <p class="endpoint-desc">Generate and load realistic test data (clears existing data). Creates 200 transactions and 200 settlement records with known discrepancy distribution.</p>
</div>

<h3>Reconciliation</h3>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-post">POST</span>
    <span class="endpoint-path">/api/v1/reconciliation/run</span>
  </div>
  <p class="endpoint-desc">Trigger a reconciliation run. Optionally pass config overrides in the request body.</p>
  <details class="try-it"><summary>Example with config override</summary>
  <pre><code>curl -X POST /api/v1/reconciliation/run \
  -H "Content-Type: application/json" \
  -d '{"variance_tolerance_pct": 0.02, "late_settlement_days": 7}'</code></pre>
  </details>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/api/v1/reconciliation/runs</span>
  </div>
  <p class="endpoint-desc">List all reconciliation runs (ID, timestamp, status)</p>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/api/v1/reconciliation/runs/{runID}</span>
  </div>
  <p class="endpoint-desc">Get full reconciliation run details including report</p>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/api/v1/reconciliation/runs/{runID}/report</span>
  </div>
  <p class="endpoint-desc">Get only the reconciliation report (summary, breakdowns, detailed results, high-priority discrepancies)</p>
</div>

<h3>Query</h3>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/api/v1/transactions/{txnID}/reconciliation</span>
  </div>
  <p class="endpoint-desc">Get reconciliation status for a specific transaction across all runs</p>
</div>

<h3>Configuration</h3>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-get">GET</span>
    <span class="endpoint-path">/api/v1/config</span>
  </div>
  <p class="endpoint-desc">Get current reconciliation configuration</p>
</div>

<div class="endpoint">
  <div class="endpoint-header">
    <span class="badge badge-put">PUT</span>
    <span class="endpoint-path">/api/v1/config</span>
  </div>
  <p class="endpoint-desc">Update reconciliation configuration (tolerance, thresholds, FX rates)</p>
</div>

<h2>Reconciliation Statuses</h2>
<table class="status-table">
  <thead><tr><th>Status</th><th>Meaning</th></tr></thead>
  <tbody>
    <tr><td><code>matched</code></td><td>Settlement found, amounts align (or within configured tolerance)</td></tr>
    <tr><td><code>matched_with_variance</code></td><td>Settlement found, amount differs beyond tolerance threshold</td></tr>
    <tr><td><code>unsettled</code></td><td>Internal transaction exists but no corresponding settlement was found</td></tr>
    <tr><td><code>unexpected_settlement</code></td><td>Settlement record exists but no corresponding internal transaction found</td></tr>
    <tr><td><code>duplicate</code></td><td>Multiple settlement records found for the same transaction</td></tr>
  </tbody>
</table>

<h2>Matching Algorithm</h2>
<p>The reconciliation engine runs in 3 phases:</p>
<p><strong>Phase 1 — Duplicate Detection:</strong> Groups settlement records by <code>processor_name:processor_txn_id</code>. Any key with more than one settlement is flagged as <code>duplicate</code>.</p>
<p><strong>Phase 2 — Settlement Matching:</strong> Each remaining settlement is matched to an internal transaction. Primary match: <code>processor_name:processor_txn_id</code>. Fallback: <code>order_reference</code> to <code>order_id</code>. If matched, amounts are compared (with optional FX conversion and tolerance).</p>
<p><strong>Phase 3 — Unsettled Detection:</strong> Any internal transaction not matched in phases 1-2 is marked <code>unsettled</code>.</p>

<h2>Report Structure</h2>
<p>The report JSON contains:</p>
<table>
  <thead><tr><th>Field</th><th>Description</th></tr></thead>
  <tbody>
    <tr><td><code>summary</code></td><td>Aggregate counts, totals, reconciliation rate %</td></tr>
    <tr><td><code>by_currency</code></td><td>Summary breakdown per currency (MXN, COP, BRL, USD)</td></tr>
    <tr><td><code>by_country</code></td><td>Summary breakdown per country (MX, CO, BR)</td></tr>
    <tr><td><code>by_processor</code></td><td>Summary breakdown per payment processor</td></tr>
    <tr><td><code>results</code></td><td>Detailed list of every reconciliation result</td></tr>
    <tr><td><code>high_priority_discrepancies</code></td><td>Filtered list: large variances or late settlements</td></tr>
  </tbody>
</table>

<h2>Configuration Options</h2>
<table>
  <thead><tr><th>Field</th><th>Type</th><th>Default</th><th>Description</th></tr></thead>
  <tbody>
    <tr><td><code>variance_tolerance_pct</code></td><td>float</td><td>0.0</td><td>Variance % below which amounts are still "matched" (e.g., 0.02 = 2%)</td></tr>
    <tr><td><code>late_settlement_days</code></td><td>int</td><td>7</td><td>Days threshold for flagging late settlements</td></tr>
    <tr><td><code>high_priority_threshold</code></td><td>float</td><td>1000.0</td><td>Minimum variance amount to flag as high priority</td></tr>
    <tr><td><code>fx_rates</code></td><td>object</td><td>—</td><td>Static FX rates map (from currency → to currency → rate)</td></tr>
  </tbody>
</table>

</body>
</html>`

// --- Health ---

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "settlement-reconciler",
	})
}

// --- Data Ingestion ---

func (h *Handler) uploadTransactions(w http.ResponseWriter, r *http.Request) {
	var txns []models.Transaction
	if err := json.NewDecoder(r.Body).Decode(&txns); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(txns) == 0 {
		writeError(w, http.StatusBadRequest, "empty transaction list")
		return
	}
	count := h.store.AddTransactions(txns)
	writeJSON(w, http.StatusCreated, map[string]any{
		"message":  fmt.Sprintf("Uploaded %d transactions (%d new)", len(txns), count),
		"received": len(txns),
		"new":      count,
	})
}

func (h *Handler) uploadSettlements(w http.ResponseWriter, r *http.Request) {
	var recs []models.SettlementRecord
	if err := json.NewDecoder(r.Body).Decode(&recs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(recs) == 0 {
		writeError(w, http.StatusBadRequest, "empty settlement list")
		return
	}
	count := h.store.AddSettlements(recs)
	writeJSON(w, http.StatusCreated, map[string]any{
		"message":  fmt.Sprintf("Uploaded %d settlement records (%d new)", len(recs), count),
		"received": len(recs),
		"new":      count,
	})
}

// --- Reconciliation ---

func (h *Handler) triggerReconciliation(w http.ResponseWriter, r *http.Request) {
	h.runSeq++
	runID := fmt.Sprintf("RUN-%04d", h.runSeq)

	run := &models.ReconciliationRun{
		ID:        runID,
		CreatedAt: time.Now().UTC(),
		Status:    "running",
	}
	h.store.SaveRun(run)

	// Parse optional config overrides from request body.
	var cfgOverride *models.ReconciliationConfig
	if r.Body != nil && r.ContentLength > 0 {
		var cfg models.ReconciliationConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err == nil {
			cfgOverride = &cfg
		}
	}

	// Use overridden config if provided, else use default.
	rec := h.reconciler
	if cfgOverride != nil {
		if cfgOverride.VarianceTolerancePct > 0 || cfgOverride.LateSettlementDays > 0 || cfgOverride.HighPriorityThreshold > 0 {
			mergedCfg := h.config
			if cfgOverride.VarianceTolerancePct > 0 {
				mergedCfg.VarianceTolerancePct = cfgOverride.VarianceTolerancePct
			}
			if cfgOverride.LateSettlementDays > 0 {
				mergedCfg.LateSettlementDays = cfgOverride.LateSettlementDays
			}
			if cfgOverride.HighPriorityThreshold > 0 {
				mergedCfg.HighPriorityThreshold = cfgOverride.HighPriorityThreshold
			}
			rec = reconciler.New(h.store, mergedCfg)
		}
	}

	report := rec.Run(runID)
	run.Status = "completed"
	run.Report = report
	h.store.SaveRun(run)

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":  runID,
		"status":  "completed",
		"summary": report.Summary,
	})
}

func (h *Handler) listRuns(w http.ResponseWriter, _ *http.Request) {
	runs := h.store.ListRuns()
	// Return lightweight list (no full reports).
	type runSummary struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		Status    string    `json:"status"`
	}
	summaries := make([]runSummary, 0, len(runs))
	for _, r := range runs {
		summaries = append(summaries, runSummary{
			ID:        r.ID,
			CreatedAt: r.CreatedAt,
			Status:    r.Status,
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	run, ok := h.store.GetRun(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "reconciliation run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) getReport(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	run, ok := h.store.GetRun(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "reconciliation run not found")
		return
	}
	if run.Report == nil {
		writeError(w, http.StatusNotFound, "report not available yet")
		return
	}
	writeJSON(w, http.StatusOK, run.Report)
}

// --- Transaction Query ---

func (h *Handler) getTransactionReconciliation(w http.ResponseWriter, r *http.Request) {
	txnID := r.PathValue("txnID")

	// Check if transaction exists.
	_, ok := h.store.GetTransaction(txnID)
	if !ok {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	// Search through all runs for results matching this transaction.
	var matchingResults []models.ReconciliationResult
	for _, run := range h.store.ListRuns() {
		if run.Report == nil {
			continue
		}
		for _, res := range run.Report.Results {
			if res.TransactionID == txnID {
				matchingResults = append(matchingResults, res)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"transaction_id": txnID,
		"results":        matchingResults,
	})
}

// --- Configuration ---

func (h *Handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.config)
}

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg models.ReconciliationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	h.config = cfg
	h.reconciler = reconciler.New(h.store, cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Configuration updated",
		"config":  cfg,
	})
}

// --- Test Data ---

func (h *Handler) generateTestData(w http.ResponseWriter, _ *http.Request) {
	h.store.Clear()
	txns, setts := generator.GenerateTestData(42)
	h.store.AddTransactions(txns)
	h.store.AddSettlements(setts)

	writeJSON(w, http.StatusCreated, map[string]any{
		"message":      "Test data generated and loaded",
		"transactions": len(txns),
		"settlements":  len(setts),
	})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

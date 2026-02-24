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

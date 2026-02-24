package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/denys-rosario/settlement-reconciler/internal/generator"
	"github.com/denys-rosario/settlement-reconciler/internal/handler"
	"github.com/denys-rosario/settlement-reconciler/internal/models"
	"github.com/denys-rosario/settlement-reconciler/internal/reconciler"
	"github.com/denys-rosario/settlement-reconciler/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize components.
	cfg := models.DefaultConfig()
	s := store.New()
	rec := reconciler.New(s, cfg)
	h := handler.New(s, rec, cfg)

	// Register routes.
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// If --seed-data flag is passed, pre-load test data and run reconciliation.
	if len(os.Args) > 1 && os.Args[1] == "--seed-data" {
		log.Println("Seeding test data...")
		txns, setts := generator.GenerateTestData(42)
		s.AddTransactions(txns)
		s.AddSettlements(setts)
		log.Printf("Loaded %d transactions and %d settlements", len(txns), len(setts))

		// Run reconciliation and write report to testdata/.
		report := rec.Run("SEED-0001")
		run := &models.ReconciliationRun{
			ID:     "SEED-0001",
			Status: "completed",
			Report: report,
		}
		s.SaveRun(run)

		// Write report to file.
		f, err := os.Create("testdata/reconciliation_report.json")
		if err != nil {
			log.Fatalf("Failed to write report: %v", err)
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		enc.Encode(report)
		f.Close()

		// Write test data files.
		writeTxns, _ := json.MarshalIndent(txns, "", "  ")
		os.WriteFile("testdata/transactions.json", writeTxns, 0644)

		writeRecs, _ := json.MarshalIndent(setts, "", "  ")
		os.WriteFile("testdata/settlements.json", writeRecs, 0644)

		log.Printf("Reconciliation complete. Report: %d matched, %d with variance, %d unsettled, %d unexpected, %d duplicates",
			report.Summary.Matched, report.Summary.MatchedWithVariance,
			report.Summary.Unsettled, report.Summary.UnexpectedSettlements,
			report.Summary.Duplicates)
		log.Println("Test data and report written to testdata/")
	}

	// Wrap with CORS and logging middleware.
	wrapped := loggingMiddleware(corsMiddleware(mux))

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Settlement Reconciliation Service starting on %s", addr)
	log.Printf("API docs: http://localhost:%s/health", port)
	if err := http.ListenAndServe(addr, wrapped); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

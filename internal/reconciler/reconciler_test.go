package reconciler

import (
	"testing"
	"time"

	"github.com/denys-rosario/settlement-reconciler/internal/models"
	"github.com/denys-rosario/settlement-reconciler/internal/store"
)

func baseTime() time.Time {
	return time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
}

func TestPerfectMatch(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	authAt := baseTime()
	captureAt := authAt.Add(2 * time.Hour)
	settleAt := authAt.Add(48 * time.Hour)

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "PaySureMX",
		ProcessorTxnID: "PSM-001", Amount: 100.00, Currency: "MXN",
		Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt,
	}})
	s.AddSettlements([]models.SettlementRecord{{
		ID: "STL-001", ProcessorName: "PaySureMX", ProcessorTxnID: "PSM-001",
		OrderReference: "ORD-001", GrossAmount: 100.00, FeeAmount: 0,
		NetAmount: 100.00, Currency: "MXN", SettledAt: settleAt,
	}})

	report := r.Run("TEST-001")

	if report.Summary.Matched != 1 {
		t.Errorf("expected 1 matched, got %d", report.Summary.Matched)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if report.Results[0].Status != models.StatusMatched {
		t.Errorf("expected status matched, got %s", report.Results[0].Status)
	}
	if report.Results[0].VarianceAmount != 0 {
		t.Errorf("expected 0 variance, got %f", report.Results[0].VarianceAmount)
	}
}

func TestMatchedWithVariance(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	authAt := baseTime()
	captureAt := authAt.Add(2 * time.Hour)
	settleAt := authAt.Add(48 * time.Hour)

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "PaySureMX",
		ProcessorTxnID: "PSM-001", Amount: 100.00, Currency: "MXN",
		Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt,
	}})
	s.AddSettlements([]models.SettlementRecord{{
		ID: "STL-001", ProcessorName: "PaySureMX", ProcessorTxnID: "PSM-001",
		OrderReference: "ORD-001", GrossAmount: 85.00, FeeAmount: 3.00,
		NetAmount: 82.00, Currency: "MXN", SettledAt: settleAt,
	}})

	report := r.Run("TEST-002")

	if report.Summary.MatchedWithVariance != 1 {
		t.Errorf("expected 1 matched_with_variance, got %d", report.Summary.MatchedWithVariance)
	}
	if report.Results[0].VarianceAmount != -15.00 {
		t.Errorf("expected -15.00 variance, got %f", report.Results[0].VarianceAmount)
	}
}

func TestVarianceTolerance(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	cfg.VarianceTolerancePct = 0.02 // 2% tolerance
	r := New(s, cfg)

	authAt := baseTime()
	captureAt := authAt.Add(2 * time.Hour)
	settleAt := authAt.Add(48 * time.Hour)

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "PaySureMX",
		ProcessorTxnID: "PSM-001", Amount: 100.00, Currency: "MXN",
		Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt,
	}})
	// 1.5% variance — should be within 2% tolerance → matched
	s.AddSettlements([]models.SettlementRecord{{
		ID: "STL-001", ProcessorName: "PaySureMX", ProcessorTxnID: "PSM-001",
		OrderReference: "ORD-001", GrossAmount: 98.50, FeeAmount: 0,
		NetAmount: 98.50, Currency: "MXN", SettledAt: settleAt,
	}})

	report := r.Run("TEST-003")

	if report.Summary.Matched != 1 {
		t.Errorf("expected 1 matched (within tolerance), got %d matched, %d variance",
			report.Summary.Matched, report.Summary.MatchedWithVariance)
	}
}

func TestUnsettled(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	authAt := baseTime()

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "PaySureMX",
		ProcessorTxnID: "PSM-001", Amount: 250.00, Currency: "BRL",
		Country: "BR", Status: "captured", AuthorizedAt: authAt,
	}})
	// No settlements added.

	report := r.Run("TEST-004")

	if report.Summary.Unsettled != 1 {
		t.Errorf("expected 1 unsettled, got %d", report.Summary.Unsettled)
	}
	if report.Results[0].Status != models.StatusUnsettled {
		t.Errorf("expected status unsettled, got %s", report.Results[0].Status)
	}
}

func TestUnexpectedSettlement(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	settleAt := baseTime()

	// No transactions added.
	s.AddSettlements([]models.SettlementRecord{{
		ID: "STL-001", ProcessorName: "GlobalTransact", ProcessorTxnID: "GT-UNKNOWN-001",
		OrderReference: "EXT-ORD-001", GrossAmount: 500.00, FeeAmount: 12.50,
		NetAmount: 487.50, Currency: "COP", SettledAt: settleAt,
	}})

	report := r.Run("TEST-005")

	if report.Summary.UnexpectedSettlements != 1 {
		t.Errorf("expected 1 unexpected settlement, got %d", report.Summary.UnexpectedSettlements)
	}
}

func TestDuplicateSettlement(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	authAt := baseTime()
	captureAt := authAt.Add(2 * time.Hour)
	settleAt1 := authAt.Add(48 * time.Hour)
	settleAt2 := authAt.Add(72 * time.Hour)

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "LatamPay",
		ProcessorTxnID: "LP-001", Amount: 300.00, Currency: "USD",
		Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt,
	}})
	s.AddSettlements([]models.SettlementRecord{
		{
			ID: "STL-001", ProcessorName: "LatamPay", ProcessorTxnID: "LP-001",
			OrderReference: "ORD-001", GrossAmount: 300.00, FeeAmount: 0,
			NetAmount: 300.00, Currency: "USD", SettledAt: settleAt1,
		},
		{
			ID: "STL-002", ProcessorName: "LatamPay", ProcessorTxnID: "LP-001",
			OrderReference: "ORD-001", GrossAmount: 300.00, FeeAmount: 0,
			NetAmount: 300.00, Currency: "USD", SettledAt: settleAt2,
		},
	})

	report := r.Run("TEST-006")

	if report.Summary.Duplicates != 2 {
		t.Errorf("expected 2 duplicate entries, got %d", report.Summary.Duplicates)
	}
}

func TestLateSettlementFlagging(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	cfg.LateSettlementDays = 7
	r := New(s, cfg)

	authAt := baseTime()
	captureAt := authAt.Add(2 * time.Hour)
	settleAt := authAt.Add(15 * 24 * time.Hour) // 15 days later

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "PaySureMX",
		ProcessorTxnID: "PSM-001", Amount: 100.00, Currency: "MXN",
		Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt,
	}})
	s.AddSettlements([]models.SettlementRecord{{
		ID: "STL-001", ProcessorName: "PaySureMX", ProcessorTxnID: "PSM-001",
		OrderReference: "ORD-001", GrossAmount: 100.00, FeeAmount: 0,
		NetAmount: 100.00, Currency: "MXN", SettledAt: settleAt,
	}})

	report := r.Run("TEST-007")

	if len(report.HighPriority) == 0 {
		t.Error("expected late settlement to be flagged as high priority")
	}
	if report.Results[0].DaysToSettle == nil || *report.Results[0].DaysToSettle != 15 {
		t.Errorf("expected 15 days to settle, got %v", report.Results[0].DaysToSettle)
	}
}

func TestFallbackMatchByOrderID(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	authAt := baseTime()
	settleAt := authAt.Add(48 * time.Hour)

	s.AddTransactions([]models.Transaction{{
		ID: "TXN-001", OrderID: "ORD-001", ProcessorName: "PaySureMX",
		ProcessorTxnID: "PSM-001", Amount: 100.00, Currency: "MXN",
		Country: "MX", Status: "captured", AuthorizedAt: authAt,
	}})
	// Different processor txn ID but same order reference → should still match
	s.AddSettlements([]models.SettlementRecord{{
		ID: "STL-001", ProcessorName: "PaySureMX", ProcessorTxnID: "PSM-DIFFERENT",
		OrderReference: "ORD-001", GrossAmount: 100.00, FeeAmount: 0,
		NetAmount: 100.00, Currency: "MXN", SettledAt: settleAt,
	}})

	report := r.Run("TEST-008")

	if report.Summary.Matched != 1 {
		t.Errorf("expected fallback match, got matched=%d, unexpected=%d",
			report.Summary.Matched, report.Summary.UnexpectedSettlements)
	}
}

func TestFullDatasetReconciliation(t *testing.T) {
	s := store.New()
	cfg := models.DefaultConfig()
	r := New(s, cfg)

	// Use the generator for a realistic full dataset.
	// Import is not needed here since we test via the store.
	// Instead, create a minimal dataset that covers all statuses.

	authAt := baseTime()
	captureAt := authAt.Add(2 * time.Hour)
	settleAt := authAt.Add(48 * time.Hour)

	// 3 matched, 1 variance, 1 unsettled, 1 unexpected, 1 duplicate pair
	txns := []models.Transaction{
		{ID: "T1", OrderID: "O1", ProcessorName: "P1", ProcessorTxnID: "PT1", Amount: 100, Currency: "USD", Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt},
		{ID: "T2", OrderID: "O2", ProcessorName: "P1", ProcessorTxnID: "PT2", Amount: 200, Currency: "MXN", Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt},
		{ID: "T3", OrderID: "O3", ProcessorName: "P2", ProcessorTxnID: "PT3", Amount: 300, Currency: "BRL", Country: "BR", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt},
		{ID: "T4", OrderID: "O4", ProcessorName: "P2", ProcessorTxnID: "PT4", Amount: 400, Currency: "COP", Country: "CO", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt},
		{ID: "T5", OrderID: "O5", ProcessorName: "P1", ProcessorTxnID: "PT5", Amount: 500, Currency: "USD", Country: "MX", Status: "captured", AuthorizedAt: authAt, CapturedAt: &captureAt},
	}
	setts := []models.SettlementRecord{
		{ID: "S1", ProcessorName: "P1", ProcessorTxnID: "PT1", OrderReference: "O1", GrossAmount: 100, NetAmount: 100, Currency: "USD", SettledAt: settleAt},
		{ID: "S2", ProcessorName: "P1", ProcessorTxnID: "PT2", OrderReference: "O2", GrossAmount: 200, NetAmount: 200, Currency: "MXN", SettledAt: settleAt},
		{ID: "S3", ProcessorName: "P2", ProcessorTxnID: "PT3", OrderReference: "O3", GrossAmount: 300, NetAmount: 300, Currency: "BRL", SettledAt: settleAt},
		{ID: "S4", ProcessorName: "P2", ProcessorTxnID: "PT4", OrderReference: "O4", GrossAmount: 350, FeeAmount: 10, NetAmount: 340, Currency: "COP", SettledAt: settleAt}, // variance
		// T5 has no settlement (unsettled)
		{ID: "S6", ProcessorName: "P3", ProcessorTxnID: "PT-X", OrderReference: "O-X", GrossAmount: 999, NetAmount: 999, Currency: "USD", SettledAt: settleAt}, // unexpected
		// Duplicate for T1
		{ID: "S7", ProcessorName: "P1", ProcessorTxnID: "PT1", OrderReference: "O1", GrossAmount: 100, NetAmount: 100, Currency: "USD", SettledAt: settleAt},
	}

	s.AddTransactions(txns)
	s.AddSettlements(setts)

	report := r.Run("FULL-TEST")

	// T1 is part of a duplicate pair (S1 + S7 → 2 duplicates)
	// T2, T3 → matched
	// T4 → matched_with_variance (350 vs 400 = -50)
	// T5 → unsettled
	// S6 → unexpected

	if report.Summary.Duplicates != 2 {
		t.Errorf("expected 2 duplicates, got %d", report.Summary.Duplicates)
	}
	if report.Summary.Matched != 2 {
		t.Errorf("expected 2 matched, got %d", report.Summary.Matched)
	}
	if report.Summary.MatchedWithVariance != 1 {
		t.Errorf("expected 1 matched_with_variance, got %d", report.Summary.MatchedWithVariance)
	}
	if report.Summary.Unsettled != 1 {
		t.Errorf("expected 1 unsettled, got %d", report.Summary.Unsettled)
	}
	if report.Summary.UnexpectedSettlements != 1 {
		t.Errorf("expected 1 unexpected, got %d", report.Summary.UnexpectedSettlements)
	}

	// Verify breakdowns exist.
	if len(report.ByCurrency) == 0 {
		t.Error("expected currency breakdown")
	}
	if len(report.ByProcessor) == 0 {
		t.Error("expected processor breakdown")
	}
}

package models

import "time"

// ReconciliationStatus represents the result of matching a transaction.
type ReconciliationStatus string

const (
	StatusMatched              ReconciliationStatus = "matched"
	StatusMatchedWithVariance  ReconciliationStatus = "matched_with_variance"
	StatusUnsettled            ReconciliationStatus = "unsettled"
	StatusUnexpectedSettlement ReconciliationStatus = "unexpected_settlement"
	StatusDuplicate            ReconciliationStatus = "duplicate"
)

// Transaction represents an internal payment authorization/capture record.
type Transaction struct {
	ID              string    `json:"id"`
	OrderID         string    `json:"order_id"`
	ProcessorName   string    `json:"processor_name"`
	ProcessorTxnID  string    `json:"processor_txn_id"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Country         string    `json:"country"`
	Status          string    `json:"status"` // authorized, captured, failed
	AuthorizedAt    time.Time `json:"authorized_at"`
	CapturedAt      *time.Time `json:"captured_at,omitempty"`
	CustomerEmail   string    `json:"customer_email"`
	PaymentMethod   string    `json:"payment_method"`
}

// SettlementRecord represents a line item from a processor's settlement file.
type SettlementRecord struct {
	ID                string    `json:"id"`
	ProcessorName     string    `json:"processor_name"`
	ProcessorTxnID    string    `json:"processor_txn_id"`
	OrderReference    string    `json:"order_reference"`
	GrossAmount       float64   `json:"gross_amount"`
	FeeAmount         float64   `json:"fee_amount"`
	NetAmount         float64   `json:"net_amount"`
	Currency          string    `json:"currency"`
	SettledAt         time.Time `json:"settled_at"`
	SettlementBatchID string    `json:"settlement_batch_id"`
}

// ReconciliationResult holds the outcome for a single matched/unmatched record.
type ReconciliationResult struct {
	ID                  string               `json:"id"`
	TransactionID       string               `json:"transaction_id,omitempty"`
	SettlementID        string               `json:"settlement_id,omitempty"`
	ProcessorName       string               `json:"processor_name"`
	Status              ReconciliationStatus  `json:"status"`
	ExpectedAmount      float64              `json:"expected_amount"`
	SettledGrossAmount  float64              `json:"settled_gross_amount"`
	SettledNetAmount    float64              `json:"settled_net_amount"`
	FeeAmount           float64              `json:"fee_amount"`
	VarianceAmount      float64              `json:"variance_amount"`
	Currency            string               `json:"currency"`
	Country             string               `json:"country"`
	AuthorizedAt        *time.Time           `json:"authorized_at,omitempty"`
	SettledAt           *time.Time           `json:"settled_at,omitempty"`
	DaysToSettle        *int                 `json:"days_to_settle,omitempty"`
	Notes               string               `json:"notes,omitempty"`
}

// ReconciliationRun represents a single reconciliation execution.
type ReconciliationRun struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"` // pending, running, completed, failed
	Report      *ReconciliationReport `json:"report,omitempty"`
}

// ReconciliationReport holds summary and detailed results.
type ReconciliationReport struct {
	RunID       string    `json:"run_id"`
	GeneratedAt time.Time `json:"generated_at"`

	// Summary
	Summary ReportSummary `json:"summary"`

	// Breakdowns
	ByCurrency  map[string]ReportSummary `json:"by_currency"`
	ByCountry   map[string]ReportSummary `json:"by_country"`
	ByProcessor map[string]ReportSummary `json:"by_processor"`

	// Detailed results
	Results []ReconciliationResult `json:"results"`

	// High-priority discrepancies
	HighPriority []ReconciliationResult `json:"high_priority_discrepancies"`
}

// ReportSummary holds aggregate reconciliation statistics.
type ReportSummary struct {
	TotalTransactions      int     `json:"total_transactions"`
	TotalSettlements       int     `json:"total_settlements"`
	Matched                int     `json:"matched"`
	MatchedWithVariance    int     `json:"matched_with_variance"`
	Unsettled              int     `json:"unsettled"`
	UnexpectedSettlements  int     `json:"unexpected_settlements"`
	Duplicates             int     `json:"duplicates"`
	TotalExpectedAmount    float64 `json:"total_expected_amount"`
	TotalSettledGross      float64 `json:"total_settled_gross"`
	TotalSettledNet        float64 `json:"total_settled_net"`
	TotalVarianceAmount    float64 `json:"total_variance_amount"`
	TotalFees              float64 `json:"total_fees"`
	ReconciliationRate     float64 `json:"reconciliation_rate_pct"`
}

// ReconciliationConfig holds configurable matching parameters.
type ReconciliationConfig struct {
	// VarianceTolerancePct is the percentage threshold below which a variance is still "matched".
	// E.g., 0.02 means amounts within 2% are considered matched.
	VarianceTolerancePct float64 `json:"variance_tolerance_pct"`

	// LateSettlementDays flags settlements that took longer than this many days.
	LateSettlementDays int `json:"late_settlement_days"`

	// HighPriorityThreshold is the minimum variance amount to flag as high priority.
	HighPriorityThreshold float64 `json:"high_priority_threshold"`

	// FX rates for multi-currency reconciliation (from -> to -> rate).
	// E.g., "BRL" -> "USD" -> 0.20
	FXRates map[string]map[string]float64 `json:"fx_rates,omitempty"`
}

// DefaultConfig returns sensible defaults for reconciliation.
func DefaultConfig() ReconciliationConfig {
	return ReconciliationConfig{
		VarianceTolerancePct:  0.0,
		LateSettlementDays:    7,
		HighPriorityThreshold: 1000.0,
		FXRates: map[string]map[string]float64{
			"MXN": {"USD": 0.058},
			"COP": {"USD": 0.00024},
			"BRL": {"USD": 0.20},
			"USD": {"USD": 1.0},
		},
	}
}

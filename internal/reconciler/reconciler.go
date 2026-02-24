package reconciler

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/denys-rosario/settlement-reconciler/internal/models"
	"github.com/denys-rosario/settlement-reconciler/internal/store"
)

// Reconciler performs the core matching logic between internal transactions
// and processor settlement records.
type Reconciler struct {
	store  *store.Store
	config models.ReconciliationConfig
}

func New(s *store.Store, cfg models.ReconciliationConfig) *Reconciler {
	return &Reconciler{store: s, config: cfg}
}

// Run executes a full reconciliation pass and returns a report.
func (r *Reconciler) Run(runID string) *models.ReconciliationReport {
	transactions := r.store.ListTransactions()
	settlements := r.store.ListSettlements()

	// Build lookup indexes for matching.
	// Primary key: processor_name:processor_txn_id
	// Fallback key: order_id / order_reference
	txnByProcessorKey := make(map[string]models.Transaction, len(transactions))
	txnByOrderID := make(map[string]models.Transaction, len(transactions))
	for _, t := range transactions {
		pk := processorKey(t.ProcessorName, t.ProcessorTxnID)
		txnByProcessorKey[pk] = t
		txnByOrderID[t.OrderID] = t
	}

	// Track which transactions and settlements have been matched.
	matchedTxnIDs := make(map[string]bool)
	matchedSettlementIDs := make(map[string]bool)

	// Track settlement processor keys to detect duplicates.
	settlementsByKey := make(map[string][]models.SettlementRecord)
	for _, s := range settlements {
		pk := processorKey(s.ProcessorName, s.ProcessorTxnID)
		settlementsByKey[pk] = append(settlementsByKey[pk], s)
	}

	var results []models.ReconciliationResult
	resultID := 0
	nextID := func() string {
		resultID++
		return fmt.Sprintf("RR-%s-%04d", runID, resultID)
	}

	// Phase 1: Detect duplicates — settlements with the same processor key appearing more than once.
	duplicateKeys := make(map[string]bool)
	for key, setts := range settlementsByKey {
		if len(setts) > 1 {
			duplicateKeys[key] = true
			txn, txnFound := findTransaction(key, setts[0].OrderReference, txnByProcessorKey, txnByOrderID)
			for _, s := range setts {
				res := models.ReconciliationResult{
					ID:                 nextID(),
					SettlementID:       s.ID,
					ProcessorName:      s.ProcessorName,
					Status:             models.StatusDuplicate,
					SettledGrossAmount: s.GrossAmount,
					SettledNetAmount:   s.NetAmount,
					FeeAmount:          s.FeeAmount,
					Currency:           s.Currency,
					Notes:              fmt.Sprintf("Duplicate settlement for processor key %s (%d occurrences)", key, len(setts)),
				}
				settledAt := s.SettledAt
				res.SettledAt = &settledAt
				if txnFound {
					res.TransactionID = txn.ID
					res.ExpectedAmount = txn.Amount
					res.Country = txn.Country
					res.VarianceAmount = s.GrossAmount - txn.Amount
					authAt := txn.AuthorizedAt
					res.AuthorizedAt = &authAt
					days := int(s.SettledAt.Sub(txn.AuthorizedAt).Hours() / 24)
					res.DaysToSettle = &days
					matchedTxnIDs[txn.ID] = true
				}
				matchedSettlementIDs[s.ID] = true
				results = append(results, res)
			}
		}
	}

	// Phase 2: Match settlements to transactions (skip duplicates already handled).
	for _, s := range settlements {
		if matchedSettlementIDs[s.ID] {
			continue
		}
		pk := processorKey(s.ProcessorName, s.ProcessorTxnID)
		if duplicateKeys[pk] {
			continue
		}

		txn, found := findTransaction(pk, s.OrderReference, txnByProcessorKey, txnByOrderID)
		if !found {
			// Unexpected settlement — no internal transaction found.
			settledAt := s.SettledAt
			results = append(results, models.ReconciliationResult{
				ID:                 nextID(),
				SettlementID:       s.ID,
				ProcessorName:      s.ProcessorName,
				Status:             models.StatusUnexpectedSettlement,
				SettledGrossAmount: s.GrossAmount,
				SettledNetAmount:   s.NetAmount,
				FeeAmount:          s.FeeAmount,
				VarianceAmount:     s.GrossAmount,
				Currency:           s.Currency,
				SettledAt:          &settledAt,
				Notes:              "Settlement record has no matching internal transaction",
			})
			matchedSettlementIDs[s.ID] = true
			continue
		}

		// We have a match — determine if amounts align.
		matchedTxnIDs[txn.ID] = true
		matchedSettlementIDs[s.ID] = true

		expectedAmount := r.convertAmount(txn.Amount, txn.Currency, s.Currency)
		variance := s.GrossAmount - expectedAmount

		status := models.StatusMatched
		notes := ""

		if math.Abs(variance) > 0.01 {
			// Check tolerance
			toleranceAmt := expectedAmount * r.config.VarianceTolerancePct
			if math.Abs(variance) <= toleranceAmt {
				status = models.StatusMatched
				notes = fmt.Sprintf("Variance of %.2f %s within tolerance (%.1f%%)", variance, s.Currency, r.config.VarianceTolerancePct*100)
			} else {
				status = models.StatusMatchedWithVariance
				if txn.Currency != s.Currency {
					notes = fmt.Sprintf("Cross-currency: authorized %.2f %s, settled %.2f %s (expected ~%.2f %s after FX)",
						txn.Amount, txn.Currency, s.GrossAmount, s.Currency, expectedAmount, s.Currency)
				} else if s.FeeAmount > 0 && math.Abs(variance+s.FeeAmount) < 0.01 {
					notes = fmt.Sprintf("Variance of %.2f %s matches fee deduction of %.2f", variance, s.Currency, s.FeeAmount)
					status = models.StatusMatched // fee-explained variance
				} else {
					notes = fmt.Sprintf("Amount variance: expected %.2f, settled gross %.2f (diff: %.2f %s)",
						expectedAmount, s.GrossAmount, variance, s.Currency)
				}
			}
		}

		authAt := txn.AuthorizedAt
		settledAt := s.SettledAt
		days := int(settledAt.Sub(authAt).Hours() / 24)

		if days > r.config.LateSettlementDays {
			if notes != "" {
				notes += "; "
			}
			notes += fmt.Sprintf("Late settlement: %d days (threshold: %d)", days, r.config.LateSettlementDays)
		}

		results = append(results, models.ReconciliationResult{
			ID:                 nextID(),
			TransactionID:      txn.ID,
			SettlementID:       s.ID,
			ProcessorName:      txn.ProcessorName,
			Status:             status,
			ExpectedAmount:     expectedAmount,
			SettledGrossAmount: s.GrossAmount,
			SettledNetAmount:   s.NetAmount,
			FeeAmount:          s.FeeAmount,
			VarianceAmount:     variance,
			Currency:           s.Currency,
			Country:            txn.Country,
			AuthorizedAt:       &authAt,
			SettledAt:          &settledAt,
			DaysToSettle:       &days,
			Notes:              notes,
		})
	}

	// Phase 3: Unsettled — internal transactions with no settlement match.
	for _, txn := range transactions {
		if matchedTxnIDs[txn.ID] {
			continue
		}
		authAt := txn.AuthorizedAt
		results = append(results, models.ReconciliationResult{
			ID:             nextID(),
			TransactionID:  txn.ID,
			ProcessorName:  txn.ProcessorName,
			Status:         models.StatusUnsettled,
			ExpectedAmount: txn.Amount,
			Currency:       txn.Currency,
			Country:        txn.Country,
			AuthorizedAt:   &authAt,
			Notes:          "No settlement record found for this transaction",
		})
	}

	// Build the report.
	report := r.buildReport(runID, transactions, settlements, results)
	return report
}

// buildReport computes summary statistics and breakdowns from the results.
func (r *Reconciler) buildReport(runID string, txns []models.Transaction, setts []models.SettlementRecord, results []models.ReconciliationResult) *models.ReconciliationReport {
	report := &models.ReconciliationReport{
		RunID:       runID,
		GeneratedAt: time.Now().UTC(),
		ByCurrency:  make(map[string]models.ReportSummary),
		ByCountry:   make(map[string]models.ReportSummary),
		ByProcessor: make(map[string]models.ReportSummary),
		Results:     results,
	}

	report.Summary.TotalTransactions = len(txns)
	report.Summary.TotalSettlements = len(setts)

	for _, res := range results {
		addToSummary(&report.Summary, res)

		if res.Currency != "" {
			s := report.ByCurrency[res.Currency]
			addToSummary(&s, res)
			report.ByCurrency[res.Currency] = s
		}
		if res.Country != "" {
			s := report.ByCountry[res.Country]
			addToSummary(&s, res)
			report.ByCountry[res.Country] = s
		}
		if res.ProcessorName != "" {
			s := report.ByProcessor[res.ProcessorName]
			addToSummary(&s, res)
			report.ByProcessor[res.ProcessorName] = s
		}

		// Flag high-priority discrepancies.
		if res.Status != models.StatusMatched && math.Abs(res.VarianceAmount) >= r.config.HighPriorityThreshold {
			report.HighPriority = append(report.HighPriority, res)
		}
		if res.DaysToSettle != nil && *res.DaysToSettle > r.config.LateSettlementDays {
			// Only add if not already high-priority.
			alreadyAdded := false
			for _, hp := range report.HighPriority {
				if hp.ID == res.ID {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				report.HighPriority = append(report.HighPriority, res)
			}
		}
	}

	// Compute reconciliation rate.
	total := report.Summary.Matched + report.Summary.MatchedWithVariance +
		report.Summary.Unsettled + report.Summary.UnexpectedSettlements + report.Summary.Duplicates
	if total > 0 {
		report.Summary.ReconciliationRate = float64(report.Summary.Matched+report.Summary.MatchedWithVariance) / float64(total) * 100
	}

	// Sort high-priority by absolute variance descending.
	sort.Slice(report.HighPriority, func(i, j int) bool {
		return math.Abs(report.HighPriority[i].VarianceAmount) > math.Abs(report.HighPriority[j].VarianceAmount)
	})

	return report
}

func addToSummary(s *models.ReportSummary, res models.ReconciliationResult) {
	switch res.Status {
	case models.StatusMatched:
		s.Matched++
	case models.StatusMatchedWithVariance:
		s.MatchedWithVariance++
	case models.StatusUnsettled:
		s.Unsettled++
	case models.StatusUnexpectedSettlement:
		s.UnexpectedSettlements++
	case models.StatusDuplicate:
		s.Duplicates++
	}
	s.TotalExpectedAmount += res.ExpectedAmount
	s.TotalSettledGross += res.SettledGrossAmount
	s.TotalSettledNet += res.SettledNetAmount
	s.TotalVarianceAmount += res.VarianceAmount
	s.TotalFees += res.FeeAmount
}

// convertAmount applies FX conversion if the currencies differ.
func (r *Reconciler) convertAmount(amount float64, from, to string) float64 {
	if from == to {
		return amount
	}
	if rates, ok := r.config.FXRates[from]; ok {
		if rate, ok := rates[to]; ok {
			return amount * rate
		}
	}
	// If no rate found, try via USD as intermediate.
	if fromUSD, ok := r.config.FXRates[from]; ok {
		if toUSD, ok := r.config.FXRates[to]; ok {
			if rateFromToUSD, ok := fromUSD["USD"]; ok {
				if rateToToUSD, ok := toUSD["USD"]; ok {
					return amount * rateFromToUSD / rateToToUSD
				}
			}
		}
	}
	return amount // fallback: no conversion
}

func processorKey(processorName, processorTxnID string) string {
	return fmt.Sprintf("%s:%s", processorName, processorTxnID)
}

// findTransaction tries primary match on processor key, then fallback on order reference.
func findTransaction(pk, orderRef string, byPK map[string]models.Transaction, byOrder map[string]models.Transaction) (models.Transaction, bool) {
	if txn, ok := byPK[pk]; ok {
		return txn, true
	}
	if orderRef != "" {
		if txn, ok := byOrder[orderRef]; ok {
			return txn, true
		}
	}
	return models.Transaction{}, false
}

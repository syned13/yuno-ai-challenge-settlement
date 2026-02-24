package store

import (
	"fmt"
	"sync"

	"github.com/denys-rosario/settlement-reconciler/internal/models"
)

// Store is a thread-safe in-memory data store for transactions,
// settlements, and reconciliation runs.
type Store struct {
	mu           sync.RWMutex
	transactions map[string]models.Transaction   // keyed by ID
	settlements  map[string]models.SettlementRecord // keyed by ID
	runs         map[string]*models.ReconciliationRun
}

func New() *Store {
	return &Store{
		transactions: make(map[string]models.Transaction),
		settlements:  make(map[string]models.SettlementRecord),
		runs:         make(map[string]*models.ReconciliationRun),
	}
}

// --- Transactions ---

func (s *Store) AddTransactions(txns []models.Transaction) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, t := range txns {
		if _, exists := s.transactions[t.ID]; !exists {
			count++
		}
		s.transactions[t.ID] = t
	}
	return count
}

func (s *Store) GetTransaction(id string) (models.Transaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.transactions[id]
	return t, ok
}

func (s *Store) ListTransactions() []models.Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.Transaction, 0, len(s.transactions))
	for _, t := range s.transactions {
		result = append(result, t)
	}
	return result
}

// --- Settlements ---

func (s *Store) AddSettlements(recs []models.SettlementRecord) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, r := range recs {
		if _, exists := s.settlements[r.ID]; !exists {
			count++
		}
		s.settlements[r.ID] = r
	}
	return count
}

func (s *Store) GetSettlement(id string) (models.SettlementRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.settlements[id]
	return r, ok
}

func (s *Store) ListSettlements() []models.SettlementRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.SettlementRecord, 0, len(s.settlements))
	for _, r := range s.settlements {
		result = append(result, r)
	}
	return result
}

// --- Reconciliation Runs ---

func (s *Store) SaveRun(run *models.ReconciliationRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
}

func (s *Store) GetRun(id string) (*models.ReconciliationRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	return r, ok
}

func (s *Store) ListRuns() []*models.ReconciliationRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.ReconciliationRun, 0, len(s.runs))
	for _, r := range s.runs {
		result = append(result, r)
	}
	return result
}

// --- Lookup helpers used by the reconciler ---

// TransactionsByProcessorTxnID builds an index of processor_name:processor_txn_id -> Transaction.
func (s *Store) TransactionsByProcessorTxnID() map[string]models.Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string]models.Transaction, len(s.transactions))
	for _, t := range s.transactions {
		key := fmt.Sprintf("%s:%s", t.ProcessorName, t.ProcessorTxnID)
		idx[key] = t
	}
	return idx
}

// TransactionsByOrderID builds an index of order_id -> Transaction.
func (s *Store) TransactionsByOrderID() map[string]models.Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string]models.Transaction, len(s.transactions))
	for _, t := range s.transactions {
		idx[t.OrderID] = t
	}
	return idx
}

// Clear removes all data from the store.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transactions = make(map[string]models.Transaction)
	s.settlements = make(map[string]models.SettlementRecord)
	s.runs = make(map[string]*models.ReconciliationRun)
}

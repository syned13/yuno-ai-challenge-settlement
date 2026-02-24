package generator

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/denys-rosario/settlement-reconciler/internal/models"
)

var (
	processors = []string{"PaySureMX", "GlobalTransact", "LatamPay", "BrazilConnect", "AndesPago"}
	countries  = []string{"MX", "CO", "BR"}
	currencies = map[string]string{
		"MX": "MXN",
		"CO": "COP",
		"BR": "BRL",
	}
	methods = []string{"credit_card", "debit_card", "pix", "bank_transfer", "wallet"}
)

// GenerateTestData creates realistic test data with the required distribution:
//   - 200+ internal transactions
//   - ~150 perfect matches, ~20 variance, ~15 unsettled, ~10 unexpected, ~5 duplicates
func GenerateTestData(seed int64) ([]models.Transaction, []models.SettlementRecord) {
	rng := rand.New(rand.NewSource(seed))
	baseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	var transactions []models.Transaction
	var settlements []models.SettlementRecord

	txnID := 0
	settID := 0

	nextTxnID := func() string {
		txnID++
		return fmt.Sprintf("TXN-%06d", txnID)
	}
	nextSettID := func() string {
		settID++
		return fmt.Sprintf("STL-%06d", settID)
	}
	nextOrderID := func() string {
		return fmt.Sprintf("ORD-%06d", txnID)
	}

	randomAmount := func() float64 {
		// Mix of small ($5-50), medium ($50-500), large ($500-5000)
		r := rng.Float64()
		switch {
		case r < 0.4:
			return math.Round((5+rng.Float64()*45)*100) / 100
		case r < 0.8:
			return math.Round((50+rng.Float64()*450)*100) / 100
		default:
			return math.Round((500+rng.Float64()*4500)*100) / 100
		}
	}

	randomDate := func() time.Time {
		return baseDate.Add(time.Duration(rng.Intn(30)) * 24 * time.Hour).
			Add(time.Duration(rng.Intn(24)) * time.Hour).
			Add(time.Duration(rng.Intn(60)) * time.Minute)
	}

	randomProcessor := func() string {
		return processors[rng.Intn(len(processors))]
	}

	randomCountry := func() string {
		return countries[rng.Intn(len(countries))]
	}

	batchID := func(t time.Time) string {
		return fmt.Sprintf("BATCH-%s", t.Format("20060102"))
	}

	// --- 1. Generate 150 perfect matches ---
	for i := 0; i < 150; i++ {
		id := nextTxnID()
		orderID := nextOrderID()
		proc := randomProcessor()
		country := randomCountry()
		currency := currencies[country]
		amount := randomAmount()
		authDate := randomDate()
		captureDate := authDate.Add(time.Duration(rng.Intn(24)) * time.Hour)
		settleDate := captureDate.Add(time.Duration(1+rng.Intn(5)) * 24 * time.Hour)
		procTxnID := fmt.Sprintf("%s-%s", proc[:3], id)

		transactions = append(transactions, models.Transaction{
			ID:             id,
			OrderID:        orderID,
			ProcessorName:  proc,
			ProcessorTxnID: procTxnID,
			Amount:         amount,
			Currency:       currency,
			Country:        country,
			Status:         "captured",
			AuthorizedAt:   authDate,
			CapturedAt:     &captureDate,
			CustomerEmail:  fmt.Sprintf("customer%d@example.com", i+1),
			PaymentMethod:  methods[rng.Intn(len(methods))],
		})

		settlements = append(settlements, models.SettlementRecord{
			ID:                nextSettID(),
			ProcessorName:     proc,
			ProcessorTxnID:    procTxnID,
			OrderReference:    orderID,
			GrossAmount:       amount,
			FeeAmount:         0,
			NetAmount:         amount,
			Currency:          currency,
			SettledAt:         settleDate,
			SettlementBatchID: batchID(settleDate),
		})
	}

	// --- 2. Generate 20 matched with variance ---
	for i := 0; i < 20; i++ {
		id := nextTxnID()
		orderID := nextOrderID()
		proc := randomProcessor()
		country := randomCountry()
		currency := currencies[country]
		amount := randomAmount()
		authDate := randomDate()
		captureDate := authDate.Add(time.Duration(rng.Intn(24)) * time.Hour)
		settleDate := captureDate.Add(time.Duration(1+rng.Intn(5)) * 24 * time.Hour)
		procTxnID := fmt.Sprintf("%s-%s", proc[:3], id)

		transactions = append(transactions, models.Transaction{
			ID:             id,
			OrderID:        orderID,
			ProcessorName:  proc,
			ProcessorTxnID: procTxnID,
			Amount:         amount,
			Currency:       currency,
			Country:        country,
			Status:         "captured",
			AuthorizedAt:   authDate,
			CapturedAt:     &captureDate,
			CustomerEmail:  fmt.Sprintf("customer%d@example.com", 150+i+1),
			PaymentMethod:  methods[rng.Intn(len(methods))],
		})

		// Vary the settlement amount: fee deduction, partial capture, or FX difference
		varianceType := rng.Intn(3)
		var grossAmount, feeAmount float64
		var notes string
		switch varianceType {
		case 0: // Fee deduction — gross matches, but net is lower
			feePercent := 0.02 + rng.Float64()*0.03 // 2-5% fee
			feeAmount = math.Round(amount*feePercent*100) / 100
			grossAmount = amount
			notes = "fee_deduction"
		case 1: // Partial capture — gross is less than auth amount
			partialPct := 0.5 + rng.Float64()*0.4 // 50-90%
			grossAmount = math.Round(amount*partialPct*100) / 100
			feeAmount = math.Round(grossAmount*0.025*100) / 100
			notes = "partial_capture"
		case 2: // Small FX/rounding difference
			diff := (rng.Float64()*2 - 1) * amount * 0.03 // ±3%
			grossAmount = math.Round((amount+diff)*100) / 100
			feeAmount = math.Round(grossAmount*0.02*100) / 100
			notes = "fx_rounding"
		}
		_ = notes

		settlements = append(settlements, models.SettlementRecord{
			ID:                nextSettID(),
			ProcessorName:     proc,
			ProcessorTxnID:    procTxnID,
			OrderReference:    orderID,
			GrossAmount:       grossAmount,
			FeeAmount:         feeAmount,
			NetAmount:         math.Round((grossAmount-feeAmount)*100) / 100,
			Currency:          currency,
			SettledAt:         settleDate,
			SettlementBatchID: batchID(settleDate),
		})
	}

	// --- 3. Generate 15 unsettled (transaction exists, no settlement) ---
	for i := 0; i < 15; i++ {
		id := nextTxnID()
		orderID := nextOrderID()
		proc := randomProcessor()
		country := randomCountry()
		currency := currencies[country]
		amount := randomAmount()
		authDate := randomDate()

		status := "captured"
		var captureDate *time.Time
		if rng.Float64() < 0.3 {
			status = "authorized" // never captured
		} else {
			cd := authDate.Add(time.Duration(rng.Intn(24)) * time.Hour)
			captureDate = &cd
		}

		transactions = append(transactions, models.Transaction{
			ID:             id,
			OrderID:        orderID,
			ProcessorName:  proc,
			ProcessorTxnID: fmt.Sprintf("%s-%s", proc[:3], id),
			Amount:         amount,
			Currency:       currency,
			Country:        country,
			Status:         status,
			AuthorizedAt:   authDate,
			CapturedAt:     captureDate,
			CustomerEmail:  fmt.Sprintf("customer%d@example.com", 170+i+1),
			PaymentMethod:  methods[rng.Intn(len(methods))],
		})
	}

	// --- 4. Generate 10 unexpected settlements (settlement exists, no transaction) ---
	for i := 0; i < 10; i++ {
		proc := randomProcessor()
		country := randomCountry()
		currency := currencies[country]
		amount := randomAmount()
		settleDate := randomDate().Add(time.Duration(3+rng.Intn(5)) * 24 * time.Hour)

		settlements = append(settlements, models.SettlementRecord{
			ID:                nextSettID(),
			ProcessorName:     proc,
			ProcessorTxnID:    fmt.Sprintf("%s-UNKNOWN-%04d", proc[:3], i+1),
			OrderReference:    fmt.Sprintf("EXT-ORD-%04d", i+1),
			GrossAmount:       amount,
			FeeAmount:         math.Round(amount*0.025*100) / 100,
			NetAmount:         math.Round(amount*0.975*100) / 100,
			Currency:          currency,
			SettledAt:         settleDate,
			SettlementBatchID: batchID(settleDate),
		})
	}

	// --- 5. Generate 5 duplicates (same processor txn ID, multiple settlements) ---
	// Pick 5 existing transactions and create extra settlement records for them.
	for i := 0; i < 5; i++ {
		// Reuse an existing matched transaction's details.
		srcTxn := transactions[rng.Intn(150)] // from the first 150 (matched)
		settleDate := randomDate().Add(time.Duration(5+rng.Intn(10)) * 24 * time.Hour)

		settlements = append(settlements, models.SettlementRecord{
			ID:                nextSettID(),
			ProcessorName:     srcTxn.ProcessorName,
			ProcessorTxnID:    srcTxn.ProcessorTxnID,
			OrderReference:    srcTxn.OrderID,
			GrossAmount:       srcTxn.Amount,
			FeeAmount:         0,
			NetAmount:         srcTxn.Amount,
			Currency:          srcTxn.Currency,
			SettledAt:         settleDate,
			SettlementBatchID: batchID(settleDate),
		})
	}

	// --- 6. Extra transactions with USD currency to add 4th currency ---
	for i := 0; i < 15; i++ {
		id := nextTxnID()
		orderID := nextOrderID()
		proc := processors[rng.Intn(len(processors))]
		amount := randomAmount()
		authDate := randomDate()
		captureDate := authDate.Add(time.Duration(rng.Intn(24)) * time.Hour)
		settleDate := captureDate.Add(time.Duration(1+rng.Intn(5)) * 24 * time.Hour)
		procTxnID := fmt.Sprintf("%s-%s", proc[:3], id)

		// Determine country: distribute across all three, currency always USD
		country := countries[rng.Intn(len(countries))]

		transactions = append(transactions, models.Transaction{
			ID:             id,
			OrderID:        orderID,
			ProcessorName:  proc,
			ProcessorTxnID: procTxnID,
			Amount:         amount,
			Currency:       "USD",
			Country:        country,
			Status:         "captured",
			AuthorizedAt:   authDate,
			CapturedAt:     &captureDate,
			CustomerEmail:  fmt.Sprintf("customer%d@example.com", 200+i+1),
			PaymentMethod:  methods[rng.Intn(len(methods))],
		})

		settlements = append(settlements, models.SettlementRecord{
			ID:                nextSettID(),
			ProcessorName:     proc,
			ProcessorTxnID:    procTxnID,
			OrderReference:    orderID,
			GrossAmount:       amount,
			FeeAmount:         0,
			NetAmount:         amount,
			Currency:          "USD",
			SettledAt:         settleDate,
			SettlementBatchID: batchID(settleDate),
		})
	}

	return transactions, settlements
}

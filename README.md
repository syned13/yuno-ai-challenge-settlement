# Settlement Reconciliation Service

A backend service that performs automated settlement reconciliation for AuraCommerce, matching payment authorizations against actual settlements received from payment processors.

Built in Go with zero external dependencies (only the standard library).

## Quick Start

```bash
# Build and run with pre-loaded test data
go run ./cmd/server --seed-data

# Or just start an empty server
go run ./cmd/server
```

The server starts on `http://localhost:8080` (override with `PORT` env var).

## Architecture

```
cmd/server/main.go          → HTTP server entry point
internal/
  models/models.go          → Data models and configuration
  store/store.go            → Thread-safe in-memory data store
  reconciler/reconciler.go  → Core matching engine (3-phase algorithm)
  generator/generator.go    → Realistic test data generator
  handler/handler.go        → REST API handlers
testdata/
  transactions.json         → 200 internal transaction records
  settlements.json          → 200 settlement records
  reconciliation_report.json → Full reconciliation report output
```

### Reconciliation Algorithm

The matching engine runs in three phases:

1. **Duplicate Detection**: Groups settlement records by processor key (`processor_name:processor_txn_id`). Any key with >1 settlement is flagged as duplicate.

2. **Settlement Matching**: For each remaining settlement record:
   - **Primary match**: Lookup by `processor_name:processor_txn_id`
   - **Fallback match**: Lookup by `order_reference` → `order_id`
   - If matched, compare amounts (with optional FX conversion and tolerance)
   - If no match found → `unexpected_settlement`

3. **Unsettled Detection**: Any internal transaction not matched by phases 1–2 → `unsettled`

### Reconciliation Statuses

| Status | Meaning |
|--------|---------|
| `matched` | Settlement found, amounts align (or within configured tolerance) |
| `matched_with_variance` | Settlement found, amount differs beyond tolerance |
| `unsettled` | Internal transaction exists, no settlement found |
| `unexpected_settlement` | Settlement exists, no internal transaction found |
| `duplicate` | Multiple settlements for the same transaction |

## API Reference

### Health Check
```
GET /health
```

### Data Ingestion

**Upload Transactions**
```bash
curl -X POST http://localhost:8080/api/v1/transactions \
  -H "Content-Type: application/json" \
  -d @testdata/transactions.json
```

**Upload Settlements**
```bash
curl -X POST http://localhost:8080/api/v1/settlements \
  -H "Content-Type: application/json" \
  -d @testdata/settlements.json
```

**Generate Test Data** (clears existing data)
```bash
curl -X POST http://localhost:8080/api/v1/test-data/generate
```

### Reconciliation

**Trigger a Reconciliation Run**
```bash
curl -X POST http://localhost:8080/api/v1/reconciliation/run
```

Optionally pass config overrides in the body:
```bash
curl -X POST http://localhost:8080/api/v1/reconciliation/run \
  -H "Content-Type: application/json" \
  -d '{"variance_tolerance_pct": 0.02, "late_settlement_days": 7}'
```

Response includes run ID and summary statistics.

**List All Runs**
```bash
curl http://localhost:8080/api/v1/reconciliation/runs
```

**Get Full Run (with report)**
```bash
curl http://localhost:8080/api/v1/reconciliation/runs/RUN-0001
```

**Get Report Only**
```bash
curl http://localhost:8080/api/v1/reconciliation/runs/RUN-0001/report
```

### Query

**Get Reconciliation Status for a Transaction**
```bash
curl http://localhost:8080/api/v1/transactions/TXN-000001/reconciliation
```

### Configuration

**Get Current Config**
```bash
curl http://localhost:8080/api/v1/config
```

**Update Config**
```bash
curl -X PUT http://localhost:8080/api/v1/config \
  -H "Content-Type: application/json" \
  -d '{
    "variance_tolerance_pct": 0.02,
    "late_settlement_days": 7,
    "high_priority_threshold": 1000,
    "fx_rates": {
      "MXN": {"USD": 0.058},
      "COP": {"USD": 0.00024},
      "BRL": {"USD": 0.20},
      "USD": {"USD": 1.0}
    }
  }'
```

## Full Walkthrough

```bash
# 1. Start the server
go run ./cmd/server

# 2. Load test data
curl -X POST http://localhost:8080/api/v1/test-data/generate

# 3. Run reconciliation
curl -X POST http://localhost:8080/api/v1/reconciliation/run

# 4. View the report
curl http://localhost:8080/api/v1/reconciliation/runs/RUN-0001/report

# 5. Query a specific transaction
curl http://localhost:8080/api/v1/transactions/TXN-000001/reconciliation

# 6. Try with 2% tolerance — some "variance" items become "matched"
curl -X PUT http://localhost:8080/api/v1/config \
  -H "Content-Type: application/json" \
  -d '{"variance_tolerance_pct": 0.02, "late_settlement_days": 7, "high_priority_threshold": 1000, "fx_rates": {"MXN":{"USD":0.058},"COP":{"USD":0.00024},"BRL":{"USD":0.20},"USD":{"USD":1.0}}}'

curl -X POST http://localhost:8080/api/v1/reconciliation/run
```

## Report Structure

The reconciliation report (JSON) includes:

- **`summary`**: Aggregate stats — total matched, variance, unsettled, unexpected, duplicates, reconciliation rate %, total amounts
- **`by_currency`**: Breakdown by MXN, COP, BRL, USD
- **`by_country`**: Breakdown by MX, CO, BR
- **`by_processor`**: Breakdown by processor name
- **`results`**: Detailed list of every reconciliation result with transaction/settlement IDs, amounts, variance, days to settle, and notes
- **`high_priority_discrepancies`**: Filtered list of results with variance above the threshold or late settlements

## Test Data

The generator (`internal/generator/generator.go`) produces:
- **200 internal transactions** across MXN, COP, BRL, USD currencies and 5 processors
- **200 settlement records** with this distribution:
  - ~150 perfect matches (same ID, same amount)
  - ~20 matched with variance (fee deductions, partial captures, FX rounding)
  - ~15 unsettled (transaction exists, no settlement)
  - ~10 unexpected settlements (settlement exists, no transaction)
  - ~5 duplicates (multiple settlements for one transaction)

Pre-generated files are in `testdata/`.

## Stretch Goals Implemented

- **Multi-currency reconciliation**: FX conversion when auth currency differs from settlement currency, with configurable static rates
- **Configurable matching rules**: Variance tolerance percentage (e.g., 2% = amounts within 2% are "matched")
- **Time-window analysis**: Flags settlements exceeding configurable late threshold (default 7 days)
- **High-priority flagging**: Large variances and late settlements surfaced in a separate report section

## Key Assumptions

- This is an MVP/prototype — data is stored in-memory (no persistence across restarts)
- FX rates are static/configurable; a production system would use live rate feeds
- The matching algorithm prioritizes `processor_name:processor_txn_id` as primary key, falling back to `order_id`/`order_reference`
- Fee-explained variances (where the variance equals the fee amount) are treated as matched
- All amounts are assumed to be in their stated currency; cross-currency matching uses the configured FX rates

## Tech Stack

- **Go 1.24** — standard library only, no external dependencies
- **net/http** with Go 1.22+ routing patterns (`GET /path`, `POST /path`, path values)
- In-memory store with `sync.RWMutex` for thread safety

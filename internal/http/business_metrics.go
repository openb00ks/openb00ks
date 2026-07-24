package httpapi

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// businessMetrics holds domain counters for the core bookkeeping events, exported on /metrics via the
// global OpenTelemetry meter (see internal/telemetry). These complement the generic HTTP metrics with
// business-meaningful signal:
//
//   - openb00ks.transactions.posted{source} — journal entries posted, source=direct (POST
//     /transactions) or receipt (POST /receipts/:id/post, i.e. a suggested draft accepted).
//   - openb00ks.receipts.uploaded — receipts captured.
//   - openb00ks.suggestions.served — suggestion responses returned; the denominator for
//     "suggestion accept rate" = rate(transactions.posted{source="receipt"}) / rate(suggestions.served).
//
// Every method is nil-safe so handlers work even when metrics are not initialised (e.g. some tests).
type businessMetrics struct {
	transactionsPosted metric.Int64Counter
	receiptsUploaded   metric.Int64Counter
	suggestionsServed  metric.Int64Counter
}

func newBusinessMetrics() *businessMetrics {
	meter := otel.Meter("openb00ks-api")
	transactionsPosted, _ := meter.Int64Counter(
		"openb00ks.transactions.posted",
		metric.WithDescription("Journal entries posted, by source (direct|receipt)."),
	)
	receiptsUploaded, _ := meter.Int64Counter(
		"openb00ks.receipts.uploaded",
		metric.WithDescription("Receipts uploaded."),
	)
	suggestionsServed, _ := meter.Int64Counter(
		"openb00ks.suggestions.served",
		metric.WithDescription("Suggestion responses served (denominator for suggestion accept rate)."),
	)
	return &businessMetrics{
		transactionsPosted: transactionsPosted,
		receiptsUploaded:   receiptsUploaded,
		suggestionsServed:  suggestionsServed,
	}
}

// transactionPosted records one posted journal entry. source is "direct" or "receipt".
func (m *businessMetrics) transactionPosted(ctx context.Context, source string) {
	if m == nil || m.transactionsPosted == nil {
		return
	}
	m.transactionsPosted.Add(ctx, 1, metric.WithAttributes(attribute.String("source", source)))
}

// receiptUploaded records one uploaded receipt.
func (m *businessMetrics) receiptUploaded(ctx context.Context) {
	if m == nil || m.receiptsUploaded == nil {
		return
	}
	m.receiptsUploaded.Add(ctx, 1)
}

// suggestionServed records one suggestion response returned to a caller.
func (m *businessMetrics) suggestionServed(ctx context.Context) {
	if m == nil || m.suggestionsServed == nil {
		return
	}
	m.suggestionsServed.Add(ctx, 1)
}

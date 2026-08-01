package answer

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresTraceStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := NewPostgresTraceStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL trace store: %v", err)
	}
	defer store.Close()

	trace := Trace{
		ID:            newTraceID(),
		UserID:        "integration-test",
		QueryHash:     "hash",
		QueryLength:   4,
		Project:       "trace-integration-test",
		Provider:      "test",
		Status:        "succeeded",
		TotalTokens:   8,
		InputRateUSD:  2,
		PromptCostUSD: 0.01,
		TotalCostUSD:  0.01,
		DurationMS:    12,
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.Save(ctx, trace); err != nil {
		t.Fatalf("save trace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), "DELETE FROM answer_traces WHERE id = $1", trace.ID)
	})
	stored, err := store.Get(ctx, trace.ID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if stored.QueryHash != "hash" || stored.TotalTokens != 8 || stored.TotalCostUSD != 0.01 {
		t.Fatalf("unexpected stored trace: %#v", stored)
	}
	metrics, err := store.Metrics(ctx, trace.Project)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if metrics.Total != 1 || metrics.TotalTokens != 8 || metrics.TotalCostUSD != 0.01 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

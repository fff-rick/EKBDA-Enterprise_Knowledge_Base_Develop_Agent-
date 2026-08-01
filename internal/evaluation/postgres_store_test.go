package evaluation

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL evaluation store: %v", err)
	}
	defer store.Close()

	suite, err := store.CreateSuite(ctx, Suite{
		ID:              newID(),
		Name:            "postgres-" + newID(),
		DefinitionHash:  "definition",
		MinimumPassRate: 0.9,
		Cases:           []Case{{Name: "case", Query: "query", Project: "project"}},
		CreatedBy:       "integration-test",
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create suite: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), "DELETE FROM evaluation_runs WHERE suite_id = $1", suite.ID)
		_, _ = store.db.ExecContext(context.Background(), "DELETE FROM evaluation_suites WHERE id = $1", suite.ID)
	})
	run := Run{
		ID:              newID(),
		SuiteID:         suite.ID,
		SuiteName:       suite.Name,
		SuiteVersion:    suite.Version,
		DefinitionHash:  suite.DefinitionHash,
		MinimumPassRate: suite.MinimumPassRate,
		Status:          RunPending,
		GateStatus:      GatePending,
		TriggeredBy:     "integration-test",
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	claimed, ok, err := store.ClaimRun(ctx, run.ID, "integration-worker", time.Now().UTC(), time.Now().UTC().Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim run: ok=%t err=%v", ok, err)
	}
	claimed.Status = RunCompleted
	claimed.GateStatus = GatePassed
	claimed.Report = Report{Total: 1, Passed: 1, PassRate: 1}
	claimed.CompletedAt = time.Now().UTC()
	if err := store.CompleteRun(ctx, claimed, "integration-worker"); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	stored, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if stored.GateStatus != GatePassed || stored.Report.Passed != 1 {
		t.Fatalf("unexpected stored run: %#v", stored)
	}
}

package access

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPostgresAccessPolicyRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL access store: %v", err)
	}
	defer store.Close()
	service, err := New(store, ModeEnforced)
	if err != nil {
		t.Fatalf("create access service: %v", err)
	}
	project := "access-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	input := CreatePolicyInput{Project: project, Owner: "integration", Users: []string{"developer-1"}, Repositories: []string{"repo"}}
	first, err := service.CreatePolicy(ctx, input, "admin-1")
	if err != nil {
		t.Fatalf("create first policy: %v", err)
	}
	second, err := service.CreatePolicy(ctx, input, "admin-1")
	if err != nil || first.Version != 1 || second.Version != 2 {
		t.Fatalf("unexpected versions: first=%#v second=%#v err=%v", first, second, err)
	}
	latest, err := service.GetLatest(ctx, project)
	if err != nil || latest.ID != second.ID {
		t.Fatalf("get latest: %#v err=%v", latest, err)
	}
	history, err := service.ListPolicies(ctx, project, 10)
	if err != nil || len(history) != 2 || history[0].Version != 2 {
		t.Fatalf("list policies: %#v err=%v", history, err)
	}
}

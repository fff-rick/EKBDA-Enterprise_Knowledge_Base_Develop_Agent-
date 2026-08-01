package access

import (
	"context"
	"errors"
	"testing"

	"ekbda/internal/auth"
)

func TestPolicyVersionsAreImmutableAndLatestIsActive(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, ModeEnforced)
	first, err := service.CreatePolicy(ctx, CreatePolicyInput{
		Project: "Order-Service", Owner: "platform", Users: []string{"developer-1"},
		Roles: []string{"TEAM_ORDER"}, Repositories: []string{"services\\order"},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create first policy: %v", err)
	}
	second, err := service.CreatePolicy(ctx, CreatePolicyInput{
		Project: "order-service", Owner: "platform", Users: []string{"developer-2"},
		Repositories: []string{"services/order"},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create second policy: %v", err)
	}
	if first.Version != 1 || second.Version != 2 || first.DefinitionHash == second.DefinitionHash {
		t.Fatalf("unexpected revisions: first=%#v second=%#v", first, second)
	}
	policies, err := service.ListPolicies(ctx, "ORDER-SERVICE", 10)
	if err != nil || len(policies) != 2 || policies[0].Version != 2 || policies[1].Users[0] != "developer-1" {
		t.Fatalf("unexpected policy history: %#v err=%v", policies, err)
	}
	if err := service.Check(ctx, auth.Identity{UserID: "developer-1"}, "order-service", ""); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("old member must be revoked by latest policy: %v", err)
	}
}

func TestCheckSupportsUserRoleRepositoryAndAdmin(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, ModeEnforced)
	_, err := service.CreatePolicy(ctx, CreatePolicyInput{
		Project: "order-service", Owner: "platform", Users: []string{"developer-1"},
		Roles: []string{"team_order"}, Repositories: []string{".", "services/order"},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	tests := []struct {
		name       string
		identity   auth.Identity
		project    string
		repository string
		want       error
	}{
		{"user member", auth.Identity{UserID: "developer-1"}, "order-service", "services\\order", nil},
		{"role member", auth.Identity{UserID: "developer-2", Roles: []string{"TEAM_ORDER"}}, "order-service", ".", nil},
		{"other repository", auth.Identity{UserID: "developer-1"}, "order-service", "services/payment", ErrAccessDenied},
		{"missing policy", auth.Identity{UserID: "developer-1"}, "payment-service", "", ErrAccessDenied},
		{"missing project", auth.Identity{UserID: "developer-1"}, "", "", ErrAccessDenied},
		{"admin bypass", auth.Identity{UserID: "admin-1", Roles: []string{"KNOWLEDGE_ADMIN"}}, "missing", "any/repo", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := service.Check(ctx, test.identity, test.project, test.repository)
			if !errors.Is(err, test.want) || (test.want == nil && err != nil) {
				t.Fatalf("Check() error=%v want=%v", err, test.want)
			}
		})
	}
}

func TestDisabledModePreservesLocalDevelopment(t *testing.T) {
	service := newTestService(t, ModeDisabled)
	err := service.Check(context.Background(), auth.Identity{}, "", "../outside")
	if err != nil {
		t.Fatalf("disabled authorization must allow request: %v", err)
	}
}

func TestCreatePolicyRejectsUnsafeRepository(t *testing.T) {
	service := newTestService(t, ModeEnforced)
	for _, repository := range []string{"../secret", "C:/repo", "/absolute", ""} {
		_, err := service.CreatePolicy(context.Background(), CreatePolicyInput{
			Project: "order-service", Owner: "platform", Repositories: []string{repository},
		}, "admin-1")
		if !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("repository %q should be rejected: %v", repository, err)
		}
	}
}

func TestNewRejectsUnknownMode(t *testing.T) {
	if _, err := New(NewMemoryStore(), "permissive"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func newTestService(t *testing.T, mode string) *Service {
	t.Helper()
	service, err := New(NewMemoryStore(), mode)
	if err != nil {
		t.Fatalf("create access service: %v", err)
	}
	return service
}

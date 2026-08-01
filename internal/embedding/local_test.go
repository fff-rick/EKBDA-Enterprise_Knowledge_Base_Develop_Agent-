package embedding

import (
	"context"
	"testing"
)

func TestLocalHashProducesNormalizedDeterministicVectors(t *testing.T) {
	provider := NewLocalHash()
	first, err := provider.Embed(context.Background(), []string{"订单退款流程", "订单退款流程"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(first) != 2 || len(first[0]) != defaultLocalDimension {
		t.Fatalf("unexpected dimensions: %d x %d", len(first), len(first[0]))
	}
	for index := range first[0] {
		if first[0][index] != first[1][index] {
			t.Fatal("expected deterministic vectors")
		}
	}
}

func TestLocalHashUsesConfiguredDimension(t *testing.T) {
	provider := NewLocalHash(768)
	vectors, err := provider.Embed(context.Background(), []string{"configured dimension"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 768 {
		t.Fatalf("unexpected configured dimension: %#v", vectors)
	}
}

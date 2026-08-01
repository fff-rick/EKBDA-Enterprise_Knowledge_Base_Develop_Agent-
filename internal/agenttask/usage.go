package agenttask

import (
	"context"
	"sync"
)

type usageContextKey struct{}

type UsageCollector struct {
	mu               sync.Mutex
	promptTokens     int
	completionTokens int
}

func WithUsageCollector(ctx context.Context, collector *UsageCollector) context.Context {
	return context.WithValue(ctx, usageContextKey{}, collector)
}

func RecordUsage(ctx context.Context, promptTokens, completionTokens int) {
	collector, _ := ctx.Value(usageContextKey{}).(*UsageCollector)
	if collector == nil || promptTokens < 0 || completionTokens < 0 {
		return
	}
	collector.mu.Lock()
	collector.promptTokens += promptTokens
	collector.completionTokens += completionTokens
	collector.mu.Unlock()
}

func (c *UsageCollector) Snapshot(pricing Pricing) Usage {
	if c == nil {
		return Usage{}
	}
	c.mu.Lock()
	promptTokens := c.promptTokens
	completionTokens := c.completionTokens
	c.mu.Unlock()
	return Usage{
		PromptTokens: promptTokens, CompletionTokens: completionTokens,
		TotalTokens: promptTokens + completionTokens,
		CostUSD: float64(promptTokens)*pricing.InputUSDPerMillionTokens/1_000_000 +
			float64(completionTokens)*pricing.OutputUSDPerMillionTokens/1_000_000,
	}
}

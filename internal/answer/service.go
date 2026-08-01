package answer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"ekbda/internal/knowledge"
)

var (
	ErrInvalidInput     = errors.New("query and project are required")
	ErrTraceWrite       = errors.New("answer trace could not be stored")
	ErrInvalidRetention = errors.New("retention_days must be between 1 and 3650")
)

const minimumAnswerVectorScore = 0.25

type Service struct {
	knowledge *knowledge.Service
	provider  Provider
	traces    TraceStore
	pricing   Pricing
}

func NewService(knowledgeService *knowledge.Service, provider Provider, traces TraceStore, pricing ...Pricing) *Service {
	if traces == nil {
		traces = NewMemoryTraceStore()
	}
	configuredPricing := Pricing{}
	if len(pricing) > 0 {
		configuredPricing = pricing[0]
	}
	if configuredPricing.InputUSDPerMillionTokens < 0 || math.IsNaN(configuredPricing.InputUSDPerMillionTokens) || math.IsInf(configuredPricing.InputUSDPerMillionTokens, 0) {
		configuredPricing.InputUSDPerMillionTokens = 0
	}
	if configuredPricing.OutputUSDPerMillionTokens < 0 || math.IsNaN(configuredPricing.OutputUSDPerMillionTokens) || math.IsInf(configuredPricing.OutputUSDPerMillionTokens, 0) {
		configuredPricing.OutputUSDPerMillionTokens = 0
	}
	return &Service{knowledge: knowledgeService, provider: provider, traces: traces, pricing: configuredPricing}
}

func (s *Service) Ask(ctx context.Context, input Input) (response Response, err error) {
	input.Query = strings.TrimSpace(input.Query)
	input.Project = strings.TrimSpace(input.Project)
	queryDigest := sha256.Sum256([]byte(input.Query))
	trace := Trace{
		ID:            newTraceID(),
		UserID:        strings.TrimSpace(input.UserID),
		QueryHash:     hex.EncodeToString(queryDigest[:]),
		QueryLength:   utf8.RuneCountInString(input.Query),
		Project:       input.Project,
		Provider:      s.provider.Name(),
		InputRateUSD:  s.pricing.InputUSDPerMillionTokens,
		OutputRateUSD: s.pricing.OutputUSDPerMillionTokens,
		CreatedAt:     time.Now().UTC(),
	}
	startedAt := time.Now()
	defer func() {
		trace.DurationMS = time.Since(startedAt).Milliseconds()
		trace.Refused = response.Refused
		trace.RefusalReason = response.RefusalReason
		trace.CitationCount = len(response.Citations)
		trace.PromptCostUSD = float64(trace.PromptTokens) * trace.InputRateUSD / 1_000_000
		trace.CompletionCostUSD = float64(trace.CompletionTokens) * trace.OutputRateUSD / 1_000_000
		trace.TotalCostUSD = trace.PromptCostUSD + trace.CompletionCostUSD
		response.TraceID = trace.ID
		if err != nil {
			trace.Status = "failed"
		} else {
			trace.Status = "succeeded"
		}
		storeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if saveErr := s.traces.Save(storeContext, trace); saveErr != nil {
			if err == nil {
				response = Response{}
				err = fmt.Errorf("%w: %v", ErrTraceWrite, saveErr)
			}
		} else if err != nil {
			err = &TraceError{TraceID: trace.ID, Err: err}
		}
	}()
	if input.Query == "" || input.Project == "" {
		trace.ErrorCode = "invalid_input"
		return Response{}, ErrInvalidInput
	}
	if input.Limit <= 0 || input.Limit > 10 {
		input.Limit = 6
	}
	results, err := s.knowledge.Search(ctx, knowledge.SearchInput{
		Query:   input.Query,
		Project: input.Project,
		Roles:   input.Roles,
		Limit:   input.Limit,
	})
	if err != nil {
		trace.ErrorCode = "retrieval_failed"
		return Response{}, fmt.Errorf("retrieve answer evidence: %w", err)
	}
	evidence := make([]Evidence, 0, len(results))
	for _, result := range results {
		if result.KeywordScore == 0 && result.VectorScore < minimumAnswerVectorScore {
			continue
		}
		evidence = append(evidence, Evidence{
			ID:           fmt.Sprintf("E%d", len(evidence)+1),
			Snippet:      result.Snippet,
			Citation:     result.Citation,
			KeywordScore: result.KeywordScore,
			VectorScore:  result.VectorScore,
			FusionScore:  result.FusionScore,
			RerankScore:  result.RerankScore,
			Reranker:     result.Reranker,
		})
	}
	trace.EvidenceCount = len(evidence)
	if len(evidence) == 0 {
		return refusedResponse(s.provider.Name(), "insufficient_evidence"), nil
	}
	draft, err := s.provider.Generate(ctx, input.Query, evidence)
	if err != nil {
		trace.ErrorCode = "generation_failed"
		return Response{}, err
	}
	trace.PromptTokens = draft.Usage.PromptTokens
	trace.CompletionTokens = draft.Usage.CompletionTokens
	trace.TotalTokens = draft.Usage.TotalTokens
	if draft.Refused {
		reason := strings.TrimSpace(draft.RefusalReason)
		if reason == "" {
			reason = "insufficient_evidence"
		}
		return refusedResponse(s.provider.Name(), reason), nil
	}
	if strings.TrimSpace(draft.Answer) == "" || len(draft.CitationIDs) == 0 {
		return refusedResponse(s.provider.Name(), "ungrounded_answer"), nil
	}
	evidenceByID := make(map[string]Evidence, len(evidence))
	for _, item := range evidence {
		evidenceByID[item.ID] = item
	}
	citations := make([]Evidence, 0, len(draft.CitationIDs))
	seen := make(map[string]bool, len(draft.CitationIDs))
	for _, citationID := range draft.CitationIDs {
		if seen[citationID] {
			continue
		}
		item, exists := evidenceByID[citationID]
		if !exists {
			return refusedResponse(s.provider.Name(), "invalid_citation"), nil
		}
		seen[citationID] = true
		citations = append(citations, item)
	}
	return Response{
		Answer:    strings.TrimSpace(draft.Answer),
		Provider:  s.provider.Name(),
		Citations: citations,
	}, nil
}

func (s *Service) Trace(ctx context.Context, id string) (Trace, error) {
	return s.traces.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) Metrics(ctx context.Context, project string) (Metrics, error) {
	return s.traces.Metrics(ctx, project)
}

func (s *Service) PruneTraces(ctx context.Context, retentionDays int) (int64, time.Time, error) {
	if retentionDays < 1 || retentionDays > 3650 {
		return 0, time.Time{}, ErrInvalidRetention
	}
	before := time.Now().UTC().AddDate(0, 0, -retentionDays)
	deleted, err := s.traces.DeleteBefore(ctx, before)
	return deleted, before, err
}

func newTraceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}

func refusedResponse(provider, reason string) Response {
	return Response{
		Answer:        "现有企业知识不足以可靠回答该问题。",
		Refused:       true,
		RefusalReason: reason,
		Provider:      provider,
		Citations:     []Evidence{},
	}
}

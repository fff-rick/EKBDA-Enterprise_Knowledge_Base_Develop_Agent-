package knowledge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"ekbda/internal/embedding"
	"ekbda/internal/reranking"
)

var (
	ErrInvalidDocument  = errors.New("invalid knowledge document")
	ErrDocumentExists   = errors.New("knowledge document source already exists")
	ErrDocumentNotFound = errors.New("knowledge document not found")
)

type Service struct {
	repository Repository
	embeddings embedding.Provider
	reranker   reranking.Provider
}

func NewService(repository Repository, embeddingProvider embedding.Provider, rerankers ...reranking.Provider) *Service {
	reranker := reranking.Provider(reranking.NewLocal())
	if len(rerankers) > 0 && rerankers[0] != nil {
		reranker = rerankers[0]
	}
	return &Service{repository: repository, embeddings: embeddingProvider, reranker: reranker}
}

func (s *Service) Create(ctx context.Context, input CreateDocumentInput) (Document, error) {
	input, err := normalizeDocumentInput(input)
	if err != nil {
		return Document{}, ErrInvalidDocument
	}
	if _, exists, err := s.repository.FindBySource(ctx, input.Project, input.SourceURI); err != nil {
		return Document{}, err
	} else if exists {
		return Document{}, ErrDocumentExists
	}

	document := newDocument(input, contentHash(input.Content))
	if err := s.embedDocument(ctx, &document); err != nil {
		return Document{}, err
	}
	if err := s.repository.Save(ctx, document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (s *Service) Import(ctx context.Context, input ImportDocumentInput) (Document, ImportAction, error) {
	normalized, err := normalizeDocumentInput(input.CreateDocumentInput)
	if err != nil || strings.TrimSpace(input.ContentHash) == "" {
		return Document{}, "", ErrInvalidDocument
	}
	existing, found, err := s.repository.FindBySource(ctx, normalized.Project, normalized.SourceURI)
	if err != nil {
		return Document{}, "", err
	}
	if found && existing.Status != DocumentStatusDeleted && existing.ContentHash == input.ContentHash && s.hasCurrentEmbeddings(existing.Chunks) {
		return existing, ImportActionSkipped, nil
	}
	if !found {
		document := newDocument(normalized, input.ContentHash)
		if err := s.embedDocument(ctx, &document); err != nil {
			return Document{}, "", err
		}
		if err := s.repository.Save(ctx, document); err != nil {
			return Document{}, "", err
		}
		return document, ImportActionCreated, nil
	}

	existing.Title = normalized.Title
	existing.Content = normalized.Content
	existing.BusinessDomain = normalized.BusinessDomain
	existing.Classification = normalized.Classification
	existing.AllowedRoles = normalized.AllowedRoles
	existing.ContentHash = input.ContentHash
	existing.Version++
	existing.Status = DocumentStatusActive
	existing.DeletedAt = nil
	existing.Chunks = ChunkContent(normalized.Content)
	if err := s.embedDocument(ctx, &existing); err != nil {
		return Document{}, "", err
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, existing); err != nil {
		return Document{}, "", err
	}
	return existing, ImportActionUpdated, nil
}

func (s *Service) MarkMissingSources(ctx context.Context, project, sourcePrefix string, seen map[string]bool) ([]Document, error) {
	documents, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	deleted := make([]Document, 0)
	for _, document := range documents {
		if document.Project != project || !strings.HasPrefix(document.SourceURI, sourcePrefix) || seen[document.SourceURI] {
			continue
		}
		now := time.Now().UTC()
		document.Status = DocumentStatusDeleted
		document.DeletedAt = &now
		document.Version++
		document.UpdatedAt = now
		if err := s.repository.Update(ctx, document); err != nil {
			return deleted, err
		}
		deleted = append(deleted, document)
	}
	return deleted, nil
}

func (s *Service) Versions(ctx context.Context, documentID string) ([]DocumentVersion, error) {
	if strings.TrimSpace(documentID) == "" {
		return nil, ErrDocumentNotFound
	}
	return s.repository.ListVersions(ctx, documentID)
}

func (s *Service) Search(ctx context.Context, input SearchInput) ([]SearchResult, error) {
	query := strings.ToLower(strings.TrimSpace(input.Query))
	if query == "" {
		return nil, errors.New("search query is required")
	}
	if input.Limit <= 0 || input.Limit > 50 {
		input.Limit = 10
	}

	tokens := tokenize(query)
	queryVectors, err := s.embeddings.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(queryVectors) != 1 || len(queryVectors[0]) == 0 {
		return nil, errors.New("embedding provider returned an invalid query vector")
	}
	var candidates []searchCandidate
	if candidateRepository, ok := s.repository.(candidateRepository); ok {
		candidateLimit := input.Limit * 10
		if candidateLimit < 50 {
			candidateLimit = 50
		}
		if candidateLimit > 200 {
			candidateLimit = 200
		}
		candidates, err = candidateRepository.SearchCandidates(ctx, candidateSearchInput{
			Query: query, Project: input.Project, Roles: input.Roles, Tokens: tokens,
			QueryVector: queryVectors[0], EmbeddingProvider: s.embeddings.Name(), Limit: candidateLimit,
		})
		if err != nil {
			return nil, err
		}
	} else {
		candidates, err = s.applicationCandidates(ctx, query, tokens, queryVectors[0], input)
		if err != nil {
			return nil, err
		}
	}
	candidates = eligibleCandidates(candidates)
	results := fuseCandidates(candidates)
	if len(results) == 0 {
		return []SearchResult{}, nil
	}
	content := make(map[string]searchCandidate, len(candidates))
	for _, candidate := range candidates {
		content[candidateKey(candidate)] = candidate
	}
	rerankCandidates := make([]reranking.Candidate, len(results))
	for index, result := range results {
		candidate := content[resultKey(result)]
		rerankCandidates[index] = reranking.Candidate{
			Title: candidate.title, Content: candidate.content,
			KeywordScore: result.KeywordScore, VectorScore: result.VectorScore, FusionScore: result.FusionScore,
		}
	}
	reranked, err := s.reranker.Rerank(ctx, query, rerankCandidates)
	if err != nil {
		return nil, fmt.Errorf("rerank knowledge candidates: %w", err)
	}
	if len(reranked.Scores) != len(results) {
		return nil, fmt.Errorf("reranker returned %d scores for %d candidates", len(reranked.Scores), len(results))
	}
	for index := range results {
		if math.IsNaN(reranked.Scores[index]) || math.IsInf(reranked.Scores[index], 0) {
			return nil, errors.New("reranker returned a non-finite score")
		}
		results[index].RerankScore = reranked.Scores[index]
		results[index].Reranker = reranked.Provider
		results[index].Score = reranked.Scores[index]
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > input.Limit {
		results = results[:input.Limit]
	}
	return results, nil
}

func (s *Service) applicationCandidates(ctx context.Context, query string, tokens []string, queryVector []float32, input SearchInput) ([]searchCandidate, error) {
	documents, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]searchCandidate, 0)
	for _, document := range documents {
		if input.Project != "" && !strings.EqualFold(document.Project, input.Project) {
			continue
		}
		if !authorized(document, input.Roles) {
			continue
		}
		chunks := document.Chunks
		if len(chunks) == 0 {
			chunks = ChunkContent(document.Content)
		}
		for _, chunk := range chunks {
			candidates = append(candidates, searchCandidate{
				title: document.Title, content: chunk.Content,
				result: SearchResult{
					KeywordScore: relevance(document.Title, chunk.Content, query, tokens),
					VectorScore:  cosineSimilarity(queryVector, chunk.Embedding),
					Snippet:      snippet(chunk.Content, 240),
					Citation: Citation{
						DocumentID: document.ID, Title: document.Title, SourceURI: document.SourceURI,
						Version: document.Version, ChunkIndex: chunk.Index,
						StartLine: chunk.StartLine, EndLine: chunk.EndLine,
					},
				},
			})
		}
	}
	return candidates, nil
}

func eligibleCandidates(candidates []searchCandidate) []searchCandidate {
	result := candidates[:0]
	for _, candidate := range candidates {
		if candidate.result.KeywordScore > 0 || candidate.result.VectorScore >= minimumVectorScore {
			result = append(result, candidate)
		}
	}
	return result
}

func resultKey(result SearchResult) string {
	return result.Citation.DocumentID + ":" + fmt.Sprintf("%d", result.Citation.ChunkIndex)
}

const (
	minimumVectorScore = 0.15
	rrfRankConstant    = 60.0
)

type searchCandidate struct {
	result  SearchResult
	title   string
	content string
}

func fuseCandidates(candidates []searchCandidate) []SearchResult {
	keywordOrder := rankedCandidateIndexes(candidates, func(candidate searchCandidate) float64 {
		return float64(candidate.result.KeywordScore)
	}, func(candidate searchCandidate) bool {
		return candidate.result.KeywordScore > 0
	})
	vectorOrder := rankedCandidateIndexes(candidates, func(candidate searchCandidate) float64 {
		return candidate.result.VectorScore
	}, func(candidate searchCandidate) bool {
		return candidate.result.VectorScore >= minimumVectorScore
	})
	scores := make([]float64, len(candidates))
	for rank, index := range keywordOrder {
		scores[index] += 1 / (rrfRankConstant + float64(rank+1))
	}
	for rank, index := range vectorOrder {
		scores[index] += 1 / (rrfRankConstant + float64(rank+1))
	}
	results := make([]SearchResult, len(candidates))
	for index, candidate := range candidates {
		candidate.result.Score = scores[index]
		candidate.result.FusionScore = scores[index]
		results[index] = candidate.result
	}
	return results
}

func rankedCandidateIndexes(candidates []searchCandidate, score func(searchCandidate) float64, include func(searchCandidate) bool) []int {
	indexes := make([]int, 0, len(candidates))
	for index, candidate := range candidates {
		if include(candidate) {
			indexes = append(indexes, index)
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		return score(candidates[indexes[i]]) > score(candidates[indexes[j]])
	})
	return indexes
}

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftMagnitude, rightMagnitude float64
	for index := range left {
		leftValue := float64(left[index])
		rightValue := float64(right[index])
		dot += leftValue * rightValue
		leftMagnitude += leftValue * leftValue
		rightMagnitude += rightValue * rightValue
	}
	if leftMagnitude == 0 || rightMagnitude == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftMagnitude) * math.Sqrt(rightMagnitude))
}

func (s *Service) embedDocument(ctx context.Context, document *Document) error {
	texts := make([]string, len(document.Chunks))
	for index, chunk := range document.Chunks {
		texts[index] = document.Title + "\n" + chunk.Content
	}
	vectors, err := s.embeddings.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed knowledge document: %w", err)
	}
	if len(vectors) != len(document.Chunks) {
		return fmt.Errorf("embedding provider returned %d vectors for %d chunks", len(vectors), len(document.Chunks))
	}
	for index := range document.Chunks {
		if len(vectors[index]) == 0 {
			return fmt.Errorf("embedding provider returned an empty vector for chunk %d", index)
		}
		document.Chunks[index].Embedding = vectors[index]
		document.Chunks[index].EmbeddingProvider = s.embeddings.Name()
	}
	return nil
}

func (s *Service) hasCurrentEmbeddings(chunks []Chunk) bool {
	if len(chunks) == 0 {
		return false
	}
	for _, chunk := range chunks {
		if len(chunk.Embedding) == 0 || chunk.EmbeddingProvider != s.embeddings.Name() {
			return false
		}
	}
	return true
}

func validClassification(classification Classification) bool {
	return classification == ClassificationPublic || classification == ClassificationInternal || classification == ClassificationRestricted
}

func normalizeRoles(roles []string) []string {
	seen := make(map[string]struct{}, len(roles))
	normalized := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		if _, exists := seen[role]; exists {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	return normalized
}

func authorized(document Document, roles []string) bool {
	if document.Classification != ClassificationRestricted {
		return true
	}
	userRoles := make(map[string]struct{}, len(roles))
	for _, role := range normalizeRoles(roles) {
		userRoles[role] = struct{}{}
	}
	for _, role := range document.AllowedRoles {
		if _, ok := userRoles[role]; ok {
			return true
		}
	}
	return false
}

func relevance(documentTitle, chunkContent, query string, tokens []string) int {
	title := strings.ToLower(documentTitle)
	content := strings.ToLower(chunkContent)
	score := 0
	if strings.Contains(title, query) {
		score += 8
	}
	if strings.Contains(content, query) {
		score += 4
	}
	for _, token := range tokens {
		if strings.Contains(title, token) {
			score += 3
		}
		if strings.Contains(content, token) {
			score++
		}
	}
	return score
}

func normalizeDocumentInput(input CreateDocumentInput) (CreateDocumentInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	input.SourceURI = strings.TrimSpace(input.SourceURI)
	input.BusinessDomain = strings.TrimSpace(input.BusinessDomain)
	input.Project = strings.TrimSpace(input.Project)
	if input.Title == "" || input.Content == "" || input.SourceURI == "" || input.Project == "" {
		return CreateDocumentInput{}, ErrInvalidDocument
	}
	if input.Classification == "" {
		input.Classification = ClassificationInternal
	}
	if !validClassification(input.Classification) {
		return CreateDocumentInput{}, ErrInvalidDocument
	}
	input.AllowedRoles = normalizeRoles(input.AllowedRoles)
	if input.Classification == ClassificationRestricted && len(input.AllowedRoles) == 0 {
		return CreateDocumentInput{}, ErrInvalidDocument
	}
	return input, nil
}

func newDocument(input CreateDocumentInput, hash string) Document {
	return Document{
		ID:             newID(),
		Title:          input.Title,
		Content:        input.Content,
		SourceURI:      input.SourceURI,
		BusinessDomain: input.BusinessDomain,
		Project:        input.Project,
		Classification: input.Classification,
		AllowedRoles:   input.AllowedRoles,
		ContentHash:    hash,
		Version:        1,
		Status:         DocumentStatusActive,
		Chunks:         ChunkContent(input.Content),
		UpdatedAt:      time.Now().UTC(),
	}
}

func contentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func tokenize(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
}

func snippet(content string, limit int) string {
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "…"
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("cryptographic random source unavailable")
	}
	return hex.EncodeToString(value[:])
}

package knowledge

import "time"

type Classification string
type DocumentStatus string

const (
	ClassificationPublic     Classification = "public"
	ClassificationInternal   Classification = "internal"
	ClassificationRestricted Classification = "restricted"
)

const (
	DocumentStatusActive  DocumentStatus = "active"
	DocumentStatusDeleted DocumentStatus = "deleted"
)

type Document struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	SourceURI      string         `json:"source_uri"`
	BusinessDomain string         `json:"business_domain"`
	Project        string         `json:"project"`
	Classification Classification `json:"classification"`
	AllowedRoles   []string       `json:"allowed_roles,omitempty"`
	ContentHash    string         `json:"content_hash"`
	Version        int            `json:"version"`
	Status         DocumentStatus `json:"status"`
	DeletedAt      *time.Time     `json:"deleted_at,omitempty"`
	Chunks         []Chunk        `json:"-"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type DocumentVersion struct {
	DocumentID     string         `json:"document_id"`
	Version        int            `json:"version"`
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	SourceURI      string         `json:"source_uri"`
	BusinessDomain string         `json:"business_domain"`
	Project        string         `json:"project"`
	Classification Classification `json:"classification"`
	AllowedRoles   []string       `json:"allowed_roles,omitempty"`
	ContentHash    string         `json:"content_hash"`
	Status         DocumentStatus `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
}

type Chunk struct {
	Index             int       `json:"index"`
	Content           string    `json:"content"`
	StartLine         int       `json:"start_line"`
	EndLine           int       `json:"end_line"`
	Embedding         []float32 `json:"-"`
	EmbeddingProvider string    `json:"-"`
}

type CreateDocumentInput struct {
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	SourceURI      string         `json:"source_uri"`
	BusinessDomain string         `json:"business_domain"`
	Project        string         `json:"project"`
	Classification Classification `json:"classification"`
	AllowedRoles   []string       `json:"allowed_roles"`
}

type SearchInput struct {
	Query   string
	Project string
	Roles   []string
	Limit   int
}

type ImportDocumentInput struct {
	CreateDocumentInput
	ContentHash string
}

type ImportAction string

const (
	ImportActionCreated ImportAction = "created"
	ImportActionUpdated ImportAction = "updated"
	ImportActionSkipped ImportAction = "skipped"
	ImportActionDeleted ImportAction = "deleted"
)

type Citation struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	SourceURI  string `json:"source_uri"`
	Version    int    `json:"version"`
	ChunkIndex int    `json:"chunk_index"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
}

type SearchResult struct {
	Score        float64  `json:"score"`
	FusionScore  float64  `json:"fusion_score"`
	RerankScore  float64  `json:"rerank_score"`
	Reranker     string   `json:"reranker"`
	KeywordScore int      `json:"keyword_score"`
	VectorScore  float64  `json:"vector_score"`
	Snippet      string   `json:"snippet"`
	Citation     Citation `json:"citation"`
}

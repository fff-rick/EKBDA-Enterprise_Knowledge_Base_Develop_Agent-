package answer

import "ekbda/internal/knowledge"

type Input struct {
	Query   string   `json:"query"`
	Project string   `json:"project"`
	UserID  string   `json:"-"`
	Roles   []string `json:"-"`
	Limit   int      `json:"limit"`
}

type Evidence struct {
	ID           string             `json:"id"`
	Snippet      string             `json:"snippet"`
	Citation     knowledge.Citation `json:"citation"`
	KeywordScore int                `json:"keyword_score"`
	VectorScore  float64            `json:"vector_score"`
	FusionScore  float64            `json:"fusion_score"`
	RerankScore  float64            `json:"rerank_score"`
	Reranker     string             `json:"reranker"`
}

type Draft struct {
	Answer        string   `json:"answer"`
	Refused       bool     `json:"refused"`
	RefusalReason string   `json:"refusal_reason"`
	CitationIDs   []string `json:"citation_ids"`
	Usage         Usage    `json:"-"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	TraceID       string     `json:"trace_id"`
	Answer        string     `json:"answer"`
	Refused       bool       `json:"refused"`
	RefusalReason string     `json:"refusal_reason,omitempty"`
	Provider      string     `json:"provider"`
	Citations     []Evidence `json:"citations"`
}

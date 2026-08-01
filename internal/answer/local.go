package answer

import (
	"context"
	"strings"
)

type LocalExtractive struct{}

func NewLocalExtractive() *LocalExtractive {
	return &LocalExtractive{}
}

func (p *LocalExtractive) Name() string {
	return "local-extractive"
}

func (p *LocalExtractive) Generate(_ context.Context, _ string, evidence []Evidence) (Draft, error) {
	if len(evidence) == 0 {
		return Draft{Refused: true, RefusalReason: "insufficient_evidence"}, nil
	}
	limit := len(evidence)
	if limit > 3 {
		limit = 3
	}
	lines := make([]string, 0, limit+1)
	lines = append(lines, "根据已检索到的企业知识：")
	citationIDs := make([]string, 0, limit)
	for _, item := range evidence[:limit] {
		lines = append(lines, "- "+item.Snippet+" ["+item.ID+"]")
		citationIDs = append(citationIDs, item.ID)
	}
	return Draft{Answer: strings.Join(lines, "\n"), CitationIDs: citationIDs}, nil
}

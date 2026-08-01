package knowledge

import (
	"context"
	"sync"
)

type MemoryRepository struct {
	mu        sync.RWMutex
	documents []Document
	versions  map[string][]DocumentVersion
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{versions: make(map[string][]DocumentVersion)}
}

func (r *MemoryRepository) Save(_ context.Context, document Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.documents = append(r.documents, cloneDocument(document))
	r.versions[document.ID] = append(r.versions[document.ID], versionFromDocument(document))
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, document Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.documents {
		if r.documents[index].ID == document.ID {
			r.documents[index] = cloneDocument(document)
			r.versions[document.ID] = append(r.versions[document.ID], versionFromDocument(document))
			return nil
		}
	}
	return ErrDocumentNotFound
}

func (r *MemoryRepository) FindBySource(_ context.Context, project, sourceURI string) (Document, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, document := range r.documents {
		if document.Project == project && document.SourceURI == sourceURI {
			return cloneDocument(document), true, nil
		}
	}
	return Document{}, false, nil
}

func (r *MemoryRepository) List(_ context.Context) ([]Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	documents := make([]Document, 0, len(r.documents))
	for _, document := range r.documents {
		if document.Status != "" && document.Status != DocumentStatusActive {
			continue
		}
		documents = append(documents, cloneDocument(document))
	}
	return documents, nil
}

func (r *MemoryRepository) ListVersions(_ context.Context, documentID string) ([]DocumentVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.versions[documentID]
	result := make([]DocumentVersion, len(versions))
	for index := range versions {
		version := versions[len(versions)-1-index]
		version.AllowedRoles = append([]string(nil), version.AllowedRoles...)
		result[index] = version
	}
	return result, nil
}

func cloneDocument(document Document) Document {
	document.AllowedRoles = append([]string(nil), document.AllowedRoles...)
	document.Chunks = append([]Chunk(nil), document.Chunks...)
	for index := range document.Chunks {
		document.Chunks[index].Embedding = append([]float32(nil), document.Chunks[index].Embedding...)
	}
	return document
}

func versionFromDocument(document Document) DocumentVersion {
	return DocumentVersion{
		DocumentID:     document.ID,
		Version:        document.Version,
		Title:          document.Title,
		Content:        document.Content,
		SourceURI:      document.SourceURI,
		BusinessDomain: document.BusinessDomain,
		Project:        document.Project,
		Classification: document.Classification,
		AllowedRoles:   append([]string(nil), document.AllowedRoles...),
		ContentHash:    document.ContentHash,
		Status:         document.Status,
		CreatedAt:      document.UpdatedAt,
	}
}

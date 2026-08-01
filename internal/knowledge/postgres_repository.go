package knowledge

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/0001_create_knowledge_documents.sql
var initialSchema string

//go:embed migrations/0002_add_document_versions_and_chunks.sql
var ingestionSchema string

//go:embed migrations/0003_add_document_history.sql
var historySchema string

//go:embed migrations/0004_add_chunk_embeddings.sql
var embeddingSchema string

//go:embed migrations/0005_enable_pgvector.sql
var vectorSchema string

type PostgresRepository struct {
	db              *sql.DB
	vectorDimension int
}

func NewPostgresRepository(ctx context.Context, dsn string, dimensions ...int) (*PostgresRepository, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	vectorDimension := 384
	if len(dimensions) > 0 {
		vectorDimension = dimensions[0]
	}
	if vectorDimension < 1 || vectorDimension > 2000 {
		db.Close()
		return nil, fmt.Errorf("embedding dimension must be between 1 and 2000 for pgvector HNSW")
	}
	repository := &PostgresRepository{db: db, vectorDimension: vectorDimension}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	for _, migration := range []string{initialSchema, ingestionSchema, historySchema, embeddingSchema, vectorSchema} {
		if _, err := db.ExecContext(ctx, migration); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize PostgreSQL schema: %w", err)
		}
	}
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS knowledge_chunks_embedding_hnsw_%d
		ON knowledge_chunks USING hnsw ((embedding_vector::vector(%d)) vector_cosine_ops)
		WHERE embedding_vector IS NOT NULL AND vector_dims(embedding_vector) = %d`, vectorDimension, vectorDimension, vectorDimension)
	if _, err := db.ExecContext(ctx, indexSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize pgvector HNSW index: %w", err)
	}
	return repository, nil
}

func (r *PostgresRepository) Save(ctx context.Context, document Document) error {
	if err := validateEmbeddingDimensions(document, r.vectorDimension); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save knowledge document: %w", err)
	}
	defer tx.Rollback()
	if err := insertDocument(ctx, tx, document); err != nil {
		return err
	}
	if err := insertChunks(ctx, tx, document); err != nil {
		return err
	}
	if err := insertVersion(ctx, tx, document); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit knowledge document: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Update(ctx context.Context, document Document) error {
	if err := validateEmbeddingDimensions(document, r.vectorDimension); err != nil {
		return err
	}
	allowedRoles, err := json.Marshal(document.AllowedRoles)
	if err != nil {
		return fmt.Errorf("encode allowed roles: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update knowledge document: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE knowledge_documents
		SET title = $2, content = $3, source_uri = $4, business_domain = $5,
		    project = $6, classification = $7, allowed_roles = $8,
		    content_hash = $9, version = $10, status = $11, deleted_at = $12,
		    updated_at = $13
		WHERE id = $1`,
		document.ID,
		document.Title,
		document.Content,
		document.SourceURI,
		document.BusinessDomain,
		document.Project,
		document.Classification,
		allowedRoles,
		document.ContentHash,
		document.Version,
		document.Status,
		document.DeletedAt,
		document.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update knowledge document: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated row count: %w", err)
	}
	if rowsAffected == 0 {
		return ErrDocumentNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM knowledge_chunks WHERE document_id = $1", document.ID); err != nil {
		return fmt.Errorf("delete existing knowledge chunks: %w", err)
	}
	if err := insertChunks(ctx, tx, document); err != nil {
		return err
	}
	if err := insertVersion(ctx, tx, document); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit knowledge document update: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindBySource(ctx context.Context, project, sourceURI string) (Document, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, title, content, source_uri, business_domain, project,
		       classification, allowed_roles, content_hash, version, status,
		       deleted_at, updated_at
		FROM knowledge_documents
		WHERE project = $1 AND source_uri = $2
		ORDER BY updated_at DESC
		LIMIT 1`, project, sourceURI)
	document, err := scanDocument(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Document{}, false, nil
		}
		return Document{}, false, fmt.Errorf("find knowledge document: %w", err)
	}
	chunks, err := r.loadChunks(ctx, document.ID)
	if err != nil {
		return Document{}, false, err
	}
	document.Chunks = chunks
	return document, true, nil
}

func insertDocument(ctx context.Context, tx *sql.Tx, document Document) error {
	allowedRoles, err := json.Marshal(document.AllowedRoles)
	if err != nil {
		return fmt.Errorf("encode allowed roles: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO knowledge_documents (
			id, title, content, source_uri, business_domain, project,
			classification, allowed_roles, content_hash, version, status,
			deleted_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		document.ID,
		document.Title,
		document.Content,
		document.SourceURI,
		document.BusinessDomain,
		document.Project,
		document.Classification,
		allowedRoles,
		document.ContentHash,
		document.Version,
		document.Status,
		document.DeletedAt,
		document.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save knowledge document: %w", err)
	}
	return nil
}

func insertVersion(ctx context.Context, tx *sql.Tx, document Document) error {
	allowedRoles, err := json.Marshal(document.AllowedRoles)
	if err != nil {
		return fmt.Errorf("encode version allowed roles: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO knowledge_document_versions (
			document_id, version, title, content, source_uri, business_domain,
			project, classification, allowed_roles, content_hash, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		document.ID,
		document.Version,
		document.Title,
		document.Content,
		document.SourceURI,
		document.BusinessDomain,
		document.Project,
		document.Classification,
		allowedRoles,
		document.ContentHash,
		document.Status,
		document.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save knowledge document version: %w", err)
	}
	return nil
}

func insertChunks(ctx context.Context, tx *sql.Tx, document Document) error {
	for _, chunk := range document.Chunks {
		embedding, err := json.Marshal(chunk.Embedding)
		if err != nil {
			return fmt.Errorf("encode chunk embedding: %w", err)
		}
		embeddingVector, err := vectorLiteral(chunk.Embedding)
		if err != nil {
			return fmt.Errorf("encode pgvector chunk embedding: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO knowledge_chunks (
				document_id, document_version, chunk_index, content, start_line, end_line,
				embedding, embedding_provider, embedding_vector
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector)`,
			document.ID,
			document.Version,
			chunk.Index,
			chunk.Content,
			chunk.StartLine,
			chunk.EndLine,
			embedding,
			chunk.EmbeddingProvider,
			embeddingVector,
		)
		if err != nil {
			return fmt.Errorf("save knowledge chunk: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) SearchCandidates(ctx context.Context, input candidateSearchInput) ([]searchCandidate, error) {
	rolesJSON, err := json.Marshal(normalizeRoles(input.Roles))
	if err != nil {
		return nil, fmt.Errorf("encode search roles: %w", err)
	}
	tokensJSON, err := json.Marshal(input.Tokens)
	if err != nil {
		return nil, fmt.Errorf("encode search tokens: %w", err)
	}
	queryVector, err := vectorLiteral(input.QueryVector)
	if err != nil {
		return nil, fmt.Errorf("encode query vector: %w", err)
	}
	dimension := len(input.QueryVector)
	if dimension < 1 || dimension > 2000 {
		return nil, fmt.Errorf("query vector dimension must be between 1 and 2000")
	}
	if dimension != r.vectorDimension {
		return nil, fmt.Errorf("query vector dimension %d does not match configured pgvector dimension %d", dimension, r.vectorDimension)
	}
	limit := input.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	vectorQuery := fmt.Sprintf(`
		SELECT d.id, d.title, d.source_uri, d.version,
		       c.chunk_index, c.content, c.start_line, c.end_line,
		       1 - (c.embedding_vector::vector(%d) <=> $1::vector(%d)) AS vector_score
		FROM knowledge_chunks c
		JOIN knowledge_documents d ON d.id = c.document_id
		WHERE d.status = 'active'
		  AND ($2 = '' OR lower(d.project) = lower($2))
		  AND (d.classification <> 'restricted' OR EXISTS (
		      SELECT 1 FROM jsonb_array_elements_text(d.allowed_roles) AS allowed(role)
		      WHERE allowed.role IN (SELECT jsonb_array_elements_text($3::jsonb))
		  ))
		  AND c.embedding_provider = $4
		  AND c.embedding_vector IS NOT NULL
		  AND vector_dims(c.embedding_vector) = %d
		ORDER BY c.embedding_vector::vector(%d) <=> $1::vector(%d)
		LIMIT $5`, dimension, dimension, dimension, dimension, dimension)
	vectorRows, err := r.db.QueryContext(ctx, vectorQuery, queryVector, input.Project, string(rolesJSON), input.EmbeddingProvider, limit)
	if err != nil {
		return nil, fmt.Errorf("query pgvector candidates: %w", err)
	}
	candidates, err := scanVectorCandidates(vectorRows, input.Query, input.Tokens)
	if err != nil {
		return nil, err
	}

	keywordRows, err := r.db.QueryContext(ctx, `
		WITH authorized AS (
			SELECT d.id, d.title, d.source_uri, d.version,
			       c.chunk_index, c.content, c.start_line, c.end_line,
			       c.embedding, c.embedding_provider
			FROM knowledge_chunks c
			JOIN knowledge_documents d ON d.id = c.document_id
			WHERE d.status = 'active'
			  AND ($2 = '' OR lower(d.project) = lower($2))
			  AND (d.classification <> 'restricted' OR EXISTS (
			      SELECT 1 FROM jsonb_array_elements_text(d.allowed_roles) AS allowed(role)
			      WHERE allowed.role IN (SELECT jsonb_array_elements_text($3::jsonb))
			  ))
		), scored AS (
			SELECT *,
			       (CASE WHEN strpos(lower(title), $1) > 0 THEN 8 ELSE 0 END
			        + CASE WHEN strpos(lower(content), $1) > 0 THEN 4 ELSE 0 END
			        + COALESCE((SELECT SUM(
			            CASE WHEN strpos(lower(title), token) > 0 THEN 3 ELSE 0 END
			            + CASE WHEN strpos(lower(content), token) > 0 THEN 1 ELSE 0 END
			          ) FROM jsonb_array_elements_text($4::jsonb) AS terms(token)), 0))::integer AS keyword_score
			FROM authorized
		)
		SELECT id, title, source_uri, version, chunk_index, content, start_line, end_line,
		       embedding, embedding_provider, keyword_score
		FROM scored
		WHERE keyword_score > 0
		ORDER BY keyword_score DESC, id, chunk_index
		LIMIT $5`, input.Query, input.Project, string(rolesJSON), string(tokensJSON), limit)
	if err != nil {
		return nil, fmt.Errorf("query keyword candidates: %w", err)
	}
	keywordCandidates, err := scanKeywordCandidates(keywordRows, input.QueryVector, input.EmbeddingProvider)
	if err != nil {
		return nil, err
	}
	return mergeCandidates(candidates, keywordCandidates), nil
}

func scanVectorCandidates(rows *sql.Rows, query string, tokens []string) ([]searchCandidate, error) {
	defer rows.Close()
	result := make([]searchCandidate, 0)
	for rows.Next() {
		var candidate searchCandidate
		if err := rows.Scan(
			&candidate.result.Citation.DocumentID, &candidate.result.Citation.Title,
			&candidate.result.Citation.SourceURI, &candidate.result.Citation.Version,
			&candidate.result.Citation.ChunkIndex, &candidate.content,
			&candidate.result.Citation.StartLine, &candidate.result.Citation.EndLine,
			&candidate.result.VectorScore,
		); err != nil {
			return nil, fmt.Errorf("scan pgvector candidate: %w", err)
		}
		candidate.title = candidate.result.Citation.Title
		candidate.result.KeywordScore = relevance(candidate.title, candidate.content, query, tokens)
		candidate.result.Snippet = snippet(candidate.content, 240)
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pgvector candidates: %w", err)
	}
	return result, nil
}

func scanKeywordCandidates(rows *sql.Rows, queryVector []float32, provider string) ([]searchCandidate, error) {
	defer rows.Close()
	result := make([]searchCandidate, 0)
	for rows.Next() {
		var candidate searchCandidate
		var embeddingJSON []byte
		var embeddingProvider string
		if err := rows.Scan(
			&candidate.result.Citation.DocumentID, &candidate.result.Citation.Title,
			&candidate.result.Citation.SourceURI, &candidate.result.Citation.Version,
			&candidate.result.Citation.ChunkIndex, &candidate.content,
			&candidate.result.Citation.StartLine, &candidate.result.Citation.EndLine,
			&embeddingJSON, &embeddingProvider, &candidate.result.KeywordScore,
		); err != nil {
			return nil, fmt.Errorf("scan keyword candidate: %w", err)
		}
		candidate.title = candidate.result.Citation.Title
		candidate.result.Snippet = snippet(candidate.content, 240)
		if embeddingProvider == provider {
			var vector []float32
			if err := json.Unmarshal(embeddingJSON, &vector); err != nil {
				return nil, fmt.Errorf("decode keyword candidate embedding: %w", err)
			}
			candidate.result.VectorScore = cosineSimilarity(queryVector, vector)
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate keyword candidates: %w", err)
	}
	return result, nil
}

func mergeCandidates(primary, secondary []searchCandidate) []searchCandidate {
	result := append([]searchCandidate(nil), primary...)
	indexes := make(map[string]int, len(result))
	for index, candidate := range result {
		indexes[candidateKey(candidate)] = index
	}
	for _, candidate := range secondary {
		key := candidateKey(candidate)
		if index, exists := indexes[key]; exists {
			if candidate.result.KeywordScore > result[index].result.KeywordScore {
				result[index].result.KeywordScore = candidate.result.KeywordScore
			}
			if candidate.result.VectorScore > result[index].result.VectorScore {
				result[index].result.VectorScore = candidate.result.VectorScore
			}
			continue
		}
		indexes[key] = len(result)
		result = append(result, candidate)
	}
	return result
}

func candidateKey(candidate searchCandidate) string {
	return candidate.result.Citation.DocumentID + ":" + strconv.Itoa(candidate.result.Citation.ChunkIndex)
}

func vectorLiteral(vector []float32) (string, error) {
	if len(vector) == 0 {
		return "", fmt.Errorf("vector is empty")
	}
	var builder strings.Builder
	builder.WriteByte('[')
	for index, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", fmt.Errorf("vector contains a non-finite value")
		}
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String(), nil
}

func validateEmbeddingDimensions(document Document, dimension int) error {
	for _, chunk := range document.Chunks {
		if len(chunk.Embedding) != dimension {
			return fmt.Errorf("chunk %d embedding dimension %d does not match configured pgvector dimension %d", chunk.Index, len(chunk.Embedding), dimension)
		}
	}
	return nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Document, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, content, source_uri, business_domain, project,
		       classification, allowed_roles, content_hash, version, status,
		       deleted_at, updated_at
		FROM knowledge_documents
		WHERE status = 'active'
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list knowledge documents: %w", err)
	}
	defer rows.Close()

	documents := make([]Document, 0)
	for rows.Next() {
		document, err := scanDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("scan knowledge document: %w", err)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge documents: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close knowledge document rows: %w", err)
	}
	for index := range documents {
		chunks, err := r.loadChunks(ctx, documents[index].ID)
		if err != nil {
			return nil, err
		}
		documents[index].Chunks = chunks
	}
	return documents, nil
}

func (r *PostgresRepository) ListVersions(ctx context.Context, documentID string) ([]DocumentVersion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT document_id, version, title, content, source_uri, business_domain,
		       project, classification, allowed_roles, content_hash, status, created_at
		FROM knowledge_document_versions
		WHERE document_id = $1
		ORDER BY version DESC`, documentID)
	if err != nil {
		return nil, fmt.Errorf("list knowledge document versions: %w", err)
	}
	defer rows.Close()
	versions := make([]DocumentVersion, 0)
	for rows.Next() {
		var version DocumentVersion
		var allowedRoles []byte
		if err := rows.Scan(
			&version.DocumentID,
			&version.Version,
			&version.Title,
			&version.Content,
			&version.SourceURI,
			&version.BusinessDomain,
			&version.Project,
			&version.Classification,
			&allowedRoles,
			&version.ContentHash,
			&version.Status,
			&version.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan knowledge document version: %w", err)
		}
		if err := json.Unmarshal(allowedRoles, &version.AllowedRoles); err != nil {
			return nil, fmt.Errorf("decode version allowed roles: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge document versions: %w", err)
	}
	return versions, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDocument(row rowScanner) (Document, error) {
	var document Document
	var allowedRoles []byte
	if err := row.Scan(
		&document.ID,
		&document.Title,
		&document.Content,
		&document.SourceURI,
		&document.BusinessDomain,
		&document.Project,
		&document.Classification,
		&allowedRoles,
		&document.ContentHash,
		&document.Version,
		&document.Status,
		&document.DeletedAt,
		&document.UpdatedAt,
	); err != nil {
		return Document{}, err
	}
	if err := json.Unmarshal(allowedRoles, &document.AllowedRoles); err != nil {
		return Document{}, fmt.Errorf("decode allowed roles for document %s: %w", document.ID, err)
	}
	return document, nil
}

func (r *PostgresRepository) loadChunks(ctx context.Context, documentID string) ([]Chunk, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT chunk_index, content, start_line, end_line, embedding, embedding_provider
		FROM knowledge_chunks
		WHERE document_id = $1
		ORDER BY chunk_index`, documentID)
	if err != nil {
		return nil, fmt.Errorf("list knowledge chunks: %w", err)
	}
	defer rows.Close()
	chunks := make([]Chunk, 0)
	for rows.Next() {
		var chunk Chunk
		var embedding []byte
		if err := rows.Scan(&chunk.Index, &chunk.Content, &chunk.StartLine, &chunk.EndLine, &embedding, &chunk.EmbeddingProvider); err != nil {
			return nil, fmt.Errorf("scan knowledge chunk: %w", err)
		}
		if err := json.Unmarshal(embedding, &chunk.Embedding); err != nil {
			return nil, fmt.Errorf("decode knowledge chunk embedding: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge chunks: %w", err)
	}
	return chunks, nil
}

func (r *PostgresRepository) Close() error {
	return r.db.Close()
}

ALTER TABLE knowledge_documents
    ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0);

CREATE INDEX IF NOT EXISTS idx_knowledge_documents_source
    ON knowledge_documents (project, source_uri);

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    document_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    document_version INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    PRIMARY KEY (document_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_document
    ON knowledge_chunks (document_id, chunk_index);

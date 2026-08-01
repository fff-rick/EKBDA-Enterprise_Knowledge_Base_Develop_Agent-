ALTER TABLE knowledge_documents
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'deleted')),
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS knowledge_document_versions (
    document_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    source_uri TEXT NOT NULL,
    business_domain TEXT NOT NULL DEFAULT '',
    project TEXT NOT NULL,
    classification TEXT NOT NULL,
    allowed_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    content_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (document_id, version)
);

INSERT INTO knowledge_document_versions (
    document_id, version, title, content, source_uri, business_domain,
    project, classification, allowed_roles, content_hash, status, created_at
)
SELECT id, version, title, content, source_uri, business_domain,
       project, classification, allowed_roles, content_hash, status, updated_at
FROM knowledge_documents
ON CONFLICT (document_id, version) DO NOTHING;

CREATE TABLE IF NOT EXISTS knowledge_documents (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    source_uri TEXT NOT NULL,
    business_domain TEXT NOT NULL DEFAULT '',
    project TEXT NOT NULL,
    classification TEXT NOT NULL CHECK (classification IN ('public', 'internal', 'restricted')),
    allowed_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_knowledge_documents_project
    ON knowledge_documents (project);

CREATE INDEX IF NOT EXISTS idx_knowledge_documents_updated_at
    ON knowledge_documents (updated_at DESC);

ALTER TABLE knowledge_chunks
    ADD COLUMN IF NOT EXISTS embedding JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS embedding_provider TEXT NOT NULL DEFAULT '';

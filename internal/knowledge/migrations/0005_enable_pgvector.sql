CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE knowledge_chunks
    ADD COLUMN IF NOT EXISTS embedding_vector vector;

UPDATE knowledge_chunks
SET embedding_vector = embedding::text::vector
WHERE embedding_vector IS NULL
  AND jsonb_typeof(embedding) = 'array'
  AND jsonb_array_length(embedding) > 0;

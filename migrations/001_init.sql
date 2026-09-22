-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Documents table
CREATE TABLE IF NOT EXISTS documents (
    id          BIGSERIAL PRIMARY KEY,
    filename    TEXT        NOT NULL,
    category    TEXT        NOT NULL DEFAULT 'general',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Document chunks table
CREATE TABLE IF NOT EXISTS document_chunks (
    id             BIGSERIAL PRIMARY KEY,
    document_id    BIGINT      NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chunk_index    INT         NOT NULL,
    content        TEXT        NOT NULL,
    embedding      vector(768),
    search_vector  TSVECTOR    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_document_chunks_document_id ON document_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_document_chunks_search_vector ON document_chunks USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_documents_category ON documents(category);

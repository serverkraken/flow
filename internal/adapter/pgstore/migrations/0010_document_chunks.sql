-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE documents ADD COLUMN chunks_hash text;

CREATE TABLE document_chunks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id text NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    owner_id    text NOT NULL,
    chunk_index int  NOT NULL,
    content     text NOT NULL,
    embedding   vector(768) NOT NULL,
    UNIQUE (document_id, chunk_index)
);
CREATE INDEX document_chunks_doc   ON document_chunks (document_id);
CREATE INDEX document_chunks_owner ON document_chunks (owner_id);
CREATE INDEX document_chunks_hnsw  ON document_chunks USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE IF EXISTS document_chunks;
ALTER TABLE documents DROP COLUMN IF EXISTS chunks_hash;
-- vector extension intentionally left installed (harmless, shared).

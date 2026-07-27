-- +goose Up
CREATE TABLE document_embed_failures (
    document_id   text PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    owner_id      text        NOT NULL,
    attempts      int         NOT NULL,
    next_retry_at timestamptz NOT NULL,
    last_error    text        NOT NULL DEFAULT '',
    dead          boolean     NOT NULL DEFAULT false
);

-- +goose Down
DROP TABLE document_embed_failures;

-- +goose Up
CREATE TABLE document_links (
    src_doc_id  TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    target_path TEXT NOT NULL
);
CREATE INDEX document_links_lookup ON document_links (owner_id, target_path);
CREATE INDEX document_links_src ON document_links (src_doc_id);

-- +goose Down
DROP TABLE document_links;

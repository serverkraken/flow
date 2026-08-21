-- +goose Up
-- Browsers normalise <textarea> submits to CRLF; the handlers stored it as-is,
-- and fencedBlock only recognises "---\n" — so every web-saved card silently
-- lost its frontmatter (69 of 512 in the dev DB when found). The usecases now
-- normalise on write; this repairs what is already stored. Idempotent: rows
-- without CR are untouched, and a second run finds none.
UPDATE documents
   SET body = replace(replace(body, E'\r\n', E'\n'), E'\r', E'\n')
 WHERE body LIKE E'%\r%';

-- +goose Down
-- Intentionally empty: CRLF was never the intended form, there is nothing to
-- restore to.

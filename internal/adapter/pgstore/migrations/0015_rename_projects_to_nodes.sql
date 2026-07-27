-- +goose Up
ALTER TABLE projects RENAME TO nodes;
ALTER TABLE documents RENAME COLUMN project_id TO node_id;
ALTER TABLE work_sessions RENAME COLUMN project_id TO node_id;
ALTER TABLE project_bindings RENAME COLUMN project_id TO node_id;
DROP INDEX documents_owner_project_path;
CREATE UNIQUE INDEX documents_owner_node_path
    ON documents (owner_id, coalesce(node_id, ''), path);

-- +goose Down
ALTER TABLE project_bindings RENAME COLUMN node_id TO project_id;
ALTER TABLE work_sessions RENAME COLUMN node_id TO project_id;
ALTER TABLE documents RENAME COLUMN node_id TO project_id;
DROP INDEX documents_owner_node_path;
CREATE UNIQUE INDEX documents_owner_project_path
    ON documents (owner_id, coalesce(project_id, ''), path);
ALTER TABLE nodes RENAME TO projects;

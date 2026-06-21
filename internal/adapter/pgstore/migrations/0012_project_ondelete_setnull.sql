-- +goose Up
ALTER TABLE work_sessions DROP CONSTRAINT work_sessions_project_id_fkey;
ALTER TABLE work_sessions ADD CONSTRAINT work_sessions_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE documents DROP CONSTRAINT documents_project_id_fkey;
ALTER TABLE documents ADD CONSTRAINT documents_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE work_sessions DROP CONSTRAINT work_sessions_project_id_fkey;
ALTER TABLE work_sessions ADD CONSTRAINT work_sessions_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id);
ALTER TABLE documents DROP CONSTRAINT documents_project_id_fkey;
ALTER TABLE documents ADD CONSTRAINT documents_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id);

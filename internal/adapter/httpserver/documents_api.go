// Package httpserver implements the REST and bearer APIs.
//
// R1 Bearer-API für documents (Spec §7) inkl. /repos/{key}/note-Alias.
// Pfad-Konvention für Repo-Notes (Spec §6):
//
//	path = "repos/" + url.PathEscape(canonicalKey) + ".md", repo_key = canonicalKey
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// DocumentsAPIDeps bundles the documents API dependencies.
type DocumentsAPIDeps struct {
	Store ports.DocumentStore
	Bus   *sse.Broadcaster
}

func (d DocumentsAPIDeps) changed(userID string) {
	if d.Bus != nil {
		d.Bus.Changed(userID, "documents")
	}
}

// MountDocumentsAPI registers the documents + repo-note-alias routes on r.
func MountDocumentsAPI(r chi.Router, d DocumentsAPIDeps) {
	r.Get("/documents", d.handleList)
	r.Get("/documents/*", d.handleGet)
	r.Put("/documents/*", d.handlePut)
	r.Delete("/documents/*", d.handleDelete)
	r.Get("/repos/{key}/note", d.handleRepoNoteGet)
	r.Put("/repos/{key}/note", d.handleRepoNotePut)
}

type documentDTO struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	RepoKey   string `json:"repo_key"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

func toDocumentDTO(doc ports.Document) documentDTO {
	return documentDTO{
		Path: doc.Path, Body: doc.Body, RepoKey: doc.RepoKey,
		Version: doc.Version, UpdatedAt: doc.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// docPath extracts the wildcard document path. Multi-segment, URL-unescaped.
func docPath(r *http.Request) string {
	raw := chi.URLParam(r, "*")
	p, err := url.PathUnescape(raw)
	if err != nil {
		p = raw
	}
	p = strings.TrimPrefix(p, "/")
	if strings.HasPrefix(p, "repos/") && strings.HasSuffix(p, ".md") {
		key := strings.TrimPrefix(p, "repos/")
		key = strings.TrimSuffix(key, ".md")
		return "repos/" + url.PathEscape(key) + ".md"
	}
	return p
}

// repoNotePath is THE path convention for repo notes (Spec §6).
func repoNotePath(canonicalKey string) string {
	return "repos/" + url.PathEscape(canonicalKey) + ".md"
}

func (d DocumentsAPIDeps) handleList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			apiError(w, http.StatusUnprocessableEntity, "limit muss eine positive Zahl sein")
			return
		}
		limit = n
	}
	entries, err := d.Store.List(user.ID, q.Get("prefix"), q.Get("q"), limit)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type entryDTO struct {
		Path      string `json:"path"`
		RepoKey   string `json:"repo_key"`
		Version   int64  `json:"version"`
		UpdatedAt string `json:"updated_at"`
		Snippet   string `json:"snippet,omitempty"`
	}
	dtos := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryDTO{
			Path: e.Path, RepoKey: e.RepoKey, Version: e.Version,
			UpdatedAt: e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), Snippet: e.Snippet,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d DocumentsAPIDeps) handleGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	doc, err := d.Store.Get(user.ID, docPath(r))
	if errors.Is(err, ports.ErrDocumentNotFound) {
		apiError(w, http.StatusNotFound, "document existiert nicht")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDocumentDTO(doc))
}

func (d DocumentsAPIDeps) handlePut(w http.ResponseWriter, r *http.Request) {
	d.putDocument(w, r, docPath(r), "")
}

func (d DocumentsAPIDeps) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	if err := d.Store.Delete(user.ID, docPath(r)); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (d DocumentsAPIDeps) handleRepoNoteGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		apiError(w, http.StatusUnprocessableEntity, "canonical-key ungültig")
		return
	}
	doc, err := d.Store.GetByRepoKey(user.ID, key)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		apiError(w, http.StatusNotFound, "keine Note für dieses Repo")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDocumentDTO(doc))
}

func (d DocumentsAPIDeps) handleRepoNotePut(w http.ResponseWriter, r *http.Request) {
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		apiError(w, http.StatusUnprocessableEntity, "canonical-key ungültig")
		return
	}
	d.putDocument(w, r, repoNotePath(key), key)
}

// putDocument is the shared If-Match write path for both surfaces.
func (d DocumentsAPIDeps) putDocument(w http.ResponseWriter, r *http.Request, path, repoKey string) {
	user, _ := UserFromContext(r.Context())
	if path == "" {
		apiError(w, http.StatusUnprocessableEntity, "pfad fehlt")
		return
	}
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header fehlt (0 = neu anlegen)")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	doc, err := d.Store.Put(user.ID, path, in.Body, repoKey, expected)
	if errors.Is(err, ports.ErrDocumentVersionConflict) {
		current, gerr := d.Store.Get(user.ID, path)
		if gerr == nil {
			writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toDocumentDTO(current)})
			return
		}
		apiError(w, http.StatusPreconditionFailed, "version conflict")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toDocumentDTO(doc))
}

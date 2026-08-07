package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/artifact"
)

type artifactAPI struct{ store artifact.Store }

func (a *artifactAPI) upload(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "artifact storage is not configured", http.StatusServiceUnavailable)
		return
	}
	name := r.URL.Query().Get("name")
	if strings.TrimSpace(name) == "" {
		writeError(w, r, invalid("name query parameter is required"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, artifact.MaxSize+1)
	metadata, err := a.store.Put(r.Context(), chi.URLParam(r, "workspaceID"), name, r.Header.Get("Content-Type"), r.Body)
	if err != nil {
		if errors.Is(err, artifact.ErrInvalid) {
			writeError(w, r, invalid("invalid artifact name"))
			return
		}
		if errors.Is(err, artifact.ErrTooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "artifact exceeds 32 MiB"})
			return
		}
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, metadata)
}

func (a *artifactAPI) download(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "artifact storage is not configured", http.StatusServiceUnavailable)
		return
	}
	metadata, body, err := a.store.Open(r.Context(), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "artifactID"))
	if err != nil {
		if err == artifact.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeError(w, r, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", metadata.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(metadata.Size, 10))
	filename := strings.NewReplacer(`"`, "", `\`, "").Replace(metadata.Name)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	if _, err := io.Copy(w, body); err != nil {
		// The response may already be partially written; logging is the only
		// useful action left and the request logger records the status.
		return
	}
}

func (a *artifactAPI) remove(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "artifact storage is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := a.store.Delete(r.Context(), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "artifactID")); err != nil {
		if err == artifact.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

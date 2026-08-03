package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// workflowAPI is CRUD over saved graphs. It runs nothing: starting a workflow
// belongs to the execution package, and this one only decides what may be
// stored.
type workflowAPI struct {
	workflows *workflow.Service
}

// maxGraphBytes caps a workflow body. A graph carries node configs — AI prompts,
// HTTP bodies — so the 64 KiB the other endpoints use is too tight, while the
// node and edge counts in workflow.Validate bound the shape rather than the size.
const maxGraphBytes = 1 << 20

// workflowView is the wire shape. It is spelled out rather than returning the
// domain struct so that adding a field to workflow.Workflow is not accidentally
// a public API change.
type workflowView struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Graph       workflow.Graph `json:"graph"`
	CreatedBy   string         `json:"createdBy,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func viewOf(w workflow.Workflow) workflowView {
	return workflowView{
		ID:          w.ID,
		Name:        w.Name,
		Description: w.Description,
		Graph:       w.Graph,
		CreatedBy:   w.CreatedBy,
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	}
}

func (a *workflowAPI) listWorkflows(w http.ResponseWriter, r *http.Request) {
	stored, err := a.workflows.List(r.Context(), chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]workflowView, 0, len(stored))
	for _, one := range stored {
		views = append(views, viewOf(one))
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": views})
}

func (a *workflowAPI) createWorkflow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Graph       workflow.Graph `json:"graph"`
	}
	if err := decodeJSONLimit(r, &body, maxGraphBytes); err != nil {
		writeError(w, r, err)
		return
	}

	claims, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, r, auth.ErrNoIdentity)
		return
	}

	created, err := a.workflows.Create(r.Context(),
		chi.URLParam(r, "workspaceID"), claims.Subject, body.Name, body.Description, body.Graph)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewOf(created))
}

func (a *workflowAPI) getWorkflow(w http.ResponseWriter, r *http.Request) {
	stored, err := a.workflows.Get(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(stored))
}

// updateWorkflow applies a partial update. The body's fields are pointers so an
// absent key and an empty value stay distinguishable — the builder autosaves the
// graph alone, and that must not blank the description.
func (a *workflowAPI) updateWorkflow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		Graph       *workflow.Graph `json:"graph"`
	}
	if err := decodeJSONLimit(r, &body, maxGraphBytes); err != nil {
		writeError(w, r, err)
		return
	}

	updated, err := a.workflows.Update(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"),
		workflow.Patch{Name: body.Name, Description: body.Description, Graph: body.Graph})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(updated))
}

func (a *workflowAPI) deleteWorkflow(w http.ResponseWriter, r *http.Request) {
	err := a.workflows.Delete(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

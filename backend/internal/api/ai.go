package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/ai"
)

type aiAPI struct {
	ai *ai.Service
}

// generateWorkflow turns a description into a proposed graph and returns it
// without storing anything.
//
// That is the whole shape of the feature: the response is what the canvas
// opens with, and the user saves it through the ordinary workflow endpoint once
// they have looked at it. A generate-and-save would put a workflow in the list
// that nobody has read.
func (a *aiAPI) generateWorkflow(w http.ResponseWriter, r *http.Request) {
	if a.ai == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "AI generation is not configured"})
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if strings.TrimSpace(body.Prompt) == "" {
		writeError(w, r, invalid("prompt is required"))
		return
	}

	generated, err := a.ai.GenerateWorkflow(r.Context(), chi.URLParam(r, "workspaceID"), body.Prompt)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, generated)
}

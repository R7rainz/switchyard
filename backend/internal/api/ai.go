package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/ai"
	"github.com/R7rainz/switchyard/backend/internal/aifeedback"
	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

type aiAPI struct {
	ai       *ai.Service
	feedback *aifeedback.Service
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

// submitFeedback is separate from generation on purpose: opting in here is
// the user's explicit action, and generation remains a no-storage request.
func (a *aiAPI) submitFeedback(w http.ResponseWriter, r *http.Request) {
	if a.feedback == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "AI feedback is not configured"})
		return
	}

	var body struct {
		Consent    bool            `json:"consent"`
		Prompt     string          `json:"prompt"`
		Outcome    string          `json:"outcome"`
		Generated  ai.Generated    `json:"generated"`
		FinalGraph *workflow.Graph `json:"finalGraph"`
	}
	if err := decodeJSONLimit(r, &body, 2*maxGraphBytes); err != nil {
		writeError(w, r, err)
		return
	}
	if !body.Consent {
		writeError(w, r, invalid("feedback requires explicit consent"))
		return
	}
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, r, auth.ErrNoIdentity)
		return
	}

	err := a.feedback.Submit(r.Context(), aifeedback.Submission{
		WorkspaceID:          chi.URLParam(r, "workspaceID"),
		UserID:               claims.Subject,
		Prompt:               body.Prompt,
		Outcome:              aifeedback.Outcome(body.Outcome),
		GeneratedName:        body.Generated.Name,
		GeneratedDescription: body.Generated.Description,
		GeneratedGraph:       body.Generated.Graph,
		FinalGraph:           body.FinalGraph,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

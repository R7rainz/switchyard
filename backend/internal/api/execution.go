package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/execution"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// executionAPI starts runs and reports on them.
//
// Starting is asynchronous: a workflow calls external services and can take
// minutes, far longer than a request should be held open. POST answers 202 with
// an id, and the caller watches it.
type executionAPI struct {
	executions *execution.Service
	events     EventStream
}

// EventStream is the part of the websocket package this layer needs, declared
// here where it is consumed. Serve upgrades the request and streams a topic
// until the client goes away; it is called after authorization, never before.
type EventStream interface {
	Serve(w http.ResponseWriter, r *http.Request, topic string) error
}

// executionView is the wire shape of a run. The graph snapshot is deliberately
// not in the list — it is the largest field by far and the dashboard draws a
// row, not a canvas — but it is in the single-execution response, because that
// is the page that has to show what actually ran.
type executionView struct {
	ID         string           `json:"id"`
	WorkflowID string           `json:"workflowId,omitempty"`
	RetryOf    string           `json:"retryOf,omitempty"`
	Status     execution.Status `json:"status"`
	Trigger    string           `json:"trigger"`
	Error      string           `json:"error,omitempty"`
	StartedBy  string           `json:"startedBy,omitempty"`
	CreatedAt  time.Time        `json:"createdAt"`
	StartedAt  *time.Time       `json:"startedAt,omitempty"`
	FinishedAt *time.Time       `json:"finishedAt,omitempty"`

	// DurationMS is computed here rather than in the browser, so every client
	// agrees on what a run took.
	DurationMS *int64 `json:"durationMs,omitempty"`
}

type nodeRunView struct {
	NodeID     string           `json:"nodeId"`
	Status     execution.Status `json:"status"`
	Output     json.RawMessage  `json:"output,omitempty"`
	Error      string           `json:"error,omitempty"`
	StartedAt  *time.Time       `json:"startedAt,omitempty"`
	FinishedAt *time.Time       `json:"finishedAt,omitempty"`
	DurationMS *int64           `json:"durationMs,omitempty"`
}

func executionViewOf(run execution.Execution) executionView {
	view := executionView{
		ID:         run.ID,
		WorkflowID: run.WorkflowID,
		RetryOf:    run.RetryOf,
		Status:     run.Status,
		Trigger:    run.Trigger,
		Error:      run.Error,
		StartedBy:  run.StartedBy,
		CreatedAt:  run.CreatedAt,
		StartedAt:  timePtr(run.StartedAt),
		FinishedAt: timePtr(run.FinishedAt),
	}
	view.DurationMS = durationMS(run.StartedAt, run.FinishedAt)
	return view
}

// startExecution begins a run of one workflow.
func (a *executionAPI) startExecution(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Input json.RawMessage `json:"input"`
	}
	// An empty body is the ordinary case — "run this now" carries nothing — so
	// it is not an error, only an absent payload. Compared against zero rather
	// than tested for positive: a chunked request reports -1, and ">" would
	// silently throw its payload away.
	if r.ContentLength != 0 {
		if err := decodeJSONLimit(r, &body, maxGraphBytes); err != nil {
			writeError(w, r, err)
			return
		}
	}

	claims, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, r, auth.ErrNoIdentity)
		return
	}

	run, err := a.executions.StartWithIdempotencyKey(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID"),
		claims.Subject, execution.TriggerManual, body.Input, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	// 202, not 201: the run has been accepted, and it has not happened yet.
	writeJSON(w, http.StatusAccepted, executionViewOf(run))
}

func (a *executionAPI) retryExecution(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, r, auth.ErrNoIdentity)
		return
	}

	run, err := a.executions.Retry(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "executionID"),
		claims.Subject, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, executionViewOf(run))
}

// listExecutions returns a workspace's runs, newest first, optionally for one
// workflow.
func (a *executionAPI) listExecutions(w http.ResponseWriter, r *http.Request) {
	const maxExecutionListLimit = 500
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, r, invalid("limit must be a positive number"))
			return
		}
		if parsed > maxExecutionListLimit {
			writeError(w, r, invalid("limit must be at most 500"))
			return
		}
		limit = parsed
	}

	runs, err := a.executions.List(r.Context(),
		chi.URLParam(r, "workspaceID"), r.URL.Query().Get("workflowId"), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]executionView, 0, len(runs))
	for _, run := range runs {
		views = append(views, executionViewOf(run))
	}
	writeJSON(w, http.StatusOK, map[string]any{"executions": views})
}

// getExecution returns one run with the graph it ran and what each node did.
// This is the explainability requirement: the snapshot and the per-node results
// together are the whole answer to "what happened".
func (a *executionAPI) getExecution(w http.ResponseWriter, r *http.Request) {
	run, nodeRuns, err := a.executions.Get(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "executionID"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	nodes := make([]nodeRunView, 0, len(nodeRuns))
	for _, node := range nodeRuns {
		nodes = append(nodes, nodeRunView{
			NodeID:     node.NodeID,
			Status:     node.Status,
			Output:     node.Output,
			Error:      node.Error,
			StartedAt:  timePtr(node.StartedAt),
			FinishedAt: timePtr(node.FinishedAt),
			DurationMS: durationMS(node.StartedAt, node.FinishedAt),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"execution": executionViewOf(run),
		"graph":     graphOf(run),
		"nodes":     nodes,
	})
}

func (a *executionAPI) cancelExecution(w http.ResponseWriter, r *http.Request) {
	err := a.executions.Cancel(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "executionID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// graphOf is the snapshot the run used, which is not necessarily what the
// workflow says today — that is the point of storing it.
func graphOf(run execution.Execution) workflow.Graph { return run.Graph }

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func durationMS(from, to time.Time) *int64 {
	if from.IsZero() || to.IsZero() {
		return nil
	}
	ms := to.Sub(from).Milliseconds()
	return &ms
}

// streamExecution upgrades to a WebSocket and pushes this run's events until
// the client goes away.
//
// The execution is looked up before subscribing, and that lookup is the
// authorization. RequirePermission checked the workspace in the URL and knows
// nothing about the {executionID} beside it, so without this, read access to
// one workspace would be read access to every run in every workspace. The
// store puts the workspace in the WHERE clause, so a run belonging to someone
// else is simply not found.
func (a *executionAPI) streamExecution(w http.ResponseWriter, r *http.Request) {
	if a.events == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "event streaming is not configured"})
		return
	}

	run, _, err := a.executions.Get(r.Context(),
		chi.URLParam(r, "workspaceID"), chi.URLParam(r, "executionID"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	// Nothing is replayed, so the client must connect before it fetches.
	//
	// That ordering is the whole contract. Connect first and the fetch returns
	// either a finished run or a running one whose remaining events all arrive
	// here — both correct. Fetch first and a run that ends in the gap is a page
	// showing RUNNING with nothing left to correct it, which is a run that
	// looks hung forever.
	//
	// Keeping a replay buffer would remove the ordering requirement and add a
	// second copy of state that can disagree with the rows. The rows are the
	// record; this is a notification.
	if err := a.events.Serve(w, r, execution.Topic(run.ID)); err != nil {
		zerolog.Ctx(r.Context()).Debug().Err(err).Str("execution_id", run.ID).Msg("stream ended")
	}
}

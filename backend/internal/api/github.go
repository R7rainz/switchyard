package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/go-chi/chi/v5"
)

const githubWebhookProvider = "github"
const githubWebhookCredential = "webhook"
const maxGitHubWebhookBytes = 1 << 20

type githubWebhookAPI struct {
	workflows   *workflow.Service
	executions  *execution.Service
	credentials *credential.Service
}

// receive accepts signed pull_request deliveries. ponytail: delivery IDs are not persisted; add a table if redelivery duplicates matter.
func (a *githubWebhookAPI) receive(w http.ResponseWriter, r *http.Request) {
	workspaceID, workflowID := chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID")
	flow, err := a.workflows.Get(r.Context(), workspaceID, workflowID)
	trigger := githubTrigger(flow.Graph)
	if err != nil || trigger == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	secret, err := a.credentials.Get(r.Context(), workspaceID, githubWebhookProvider, githubWebhookCredential)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "webhook is not configured"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGitHubWebhookBytes))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "webhook payload is too large"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid webhook payload"})
		}
		return
	}
	if !githubSignatureValid(secret, r.Header.Get("X-Hub-Signature-256"), body) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid webhook signature"})
		return
	}
	if r.Header.Get("X-GitHub-Event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var delivery struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(body, &delivery) != nil || delivery.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid pull request delivery"})
		return
	}
	if delivery.Action != githubTriggerAction(trigger) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// GitHub retries deliveries with the same delivery id. Persisting it as the
	// execution key makes a redelivery return the original run instead of
	// repeating its side effects.
	key := ""
	if deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery")); deliveryID != "" {
		key = "github:" + deliveryID
	}
	run, err := a.executions.StartWithIdempotencyKey(r.Context(), workspaceID, workflowID, "", execution.TriggerWebhook, body, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, executionViewOf(run))
}

func githubTrigger(graph workflow.Graph) *workflow.Node {
	for i := range graph.Nodes {
		if graph.Nodes[i].Type == "trigger.github.pull_request" {
			return &graph.Nodes[i]
		}
	}
	return nil
}
func githubTriggerAction(node *workflow.Node) string {
	var data struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(node.Data, &data) == nil && data.Action != "" {
		return data.Action
	}
	return "opened"
}
func githubSignatureValid(secret credential.Secret, header string, body []byte) bool {
	value, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return subtle.ConstantTimeCompare(got, mac.Sum(nil)) == 1
}

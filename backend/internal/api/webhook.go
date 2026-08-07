package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

const (
	genericWebhookProvider = "webhook"
	genericWebhookName     = "default"
	maxGenericWebhookBytes = 1 << 20
)

type genericWebhookAPI struct {
	workflows   *workflow.Service
	executions  *execution.Service
	credentials *credential.Service
}

// receive accepts any JSON or text payload for a trigger.webhook workflow.
// Authentication is an HMAC over the exact request body using webhook/default.
func (a *genericWebhookAPI) receive(w http.ResponseWriter, r *http.Request) {
	workspaceID, workflowID := chi.URLParam(r, "workspaceID"), chi.URLParam(r, "workflowID")
	flow, err := a.workflows.Get(r.Context(), workspaceID, workflowID)
	if err != nil || !hasWebhookTrigger(flow.Graph) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}

	secret, err := a.credentials.Get(r.Context(), workspaceID, genericWebhookProvider, genericWebhookName)
	if errors.Is(err, credential.ErrNotFound) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "webhook is not configured"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "webhook is not configured"})
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGenericWebhookBytes))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "webhook payload is too large"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid webhook payload"})
		}
		return
	}
	if !webhookSignatureValid(secret, r.Header.Get("X-Switchyard-Signature-256"), body) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid webhook signature"})
		return
	}

	delivery := strings.TrimSpace(r.Header.Get("X-Webhook-Delivery"))
	if delivery == "" {
		delivery = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	key := ""
	if delivery != "" {
		key = "webhook:" + workflowID + ":" + delivery
	}
	run, err := a.executions.StartWithIdempotencyKey(
		r.Context(), workspaceID, workflowID, "", execution.TriggerWebhook, body, key,
	)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, executionViewOf(run))
}

func hasWebhookTrigger(graph workflow.Graph) bool {
	for _, node := range graph.Nodes {
		if node.Type == "trigger.webhook" {
			return true
		}
	}
	return false
}

func webhookSignatureValid(secret credential.Secret, header string, body []byte) bool {
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

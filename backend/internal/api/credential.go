package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

// credentialAPI exposes the workspace's stored provider secrets.
//
// **There is no endpoint that returns a secret.** A credential goes in and is
// never read back out over HTTP; the only reader is the execution engine, in
// process, through credential.Service. An endpoint that returned plaintext
// would undo the encryption at rest — the ciphertext would be safe in the
// database and the key would be one authenticated GET away.
//
// So this is write, list-metadata, and delete. Replacing a secret is how you
// "edit" one, and forgetting what a key was is the expected outcome.
type credentialAPI struct {
	credentials *credential.Service
}

// maxSecretBytes caps a stored secret. An OAuth token document is the largest
// thing that legitimately goes in here and is nowhere near this.
const maxSecretBytes = 8 << 10

// credentialView is the safe shape: what exists, not what it holds.
type credentialView struct {
	Provider  string    `json:"provider"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// putCredential stores or replaces a secret.
//
// PUT rather than POST because the workspace, provider, and name together are
// the identity: sending it twice leaves one credential, not two.
func (a *credentialAPI) putCredential(w http.ResponseWriter, r *http.Request) {
	provider, name, err := credentialPath(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body struct {
		Secret string `json:"secret"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if body.Secret == "" {
		writeError(w, r, invalid("secret is required"))
		return
	}
	if len(body.Secret) > maxSecretBytes {
		writeError(w, r, invalid("secret exceeds %d bytes", maxSecretBytes))
		return
	}

	workspaceID := chi.URLParam(r, "workspaceID")
	if err := a.credentials.Put(r.Context(), workspaceID, provider, name, credential.Secret(body.Secret)); err != nil {
		// Whatever went wrong, the secret is not in the error and must not
		// reach the response.
		writeError(w, r, err)
		return
	}

	// 204: there is nothing to return that the caller does not already have,
	// and echoing the secret back is exactly what this package will not do.
	w.WriteHeader(http.StatusNoContent)
}

// listCredentials reports which credentials the workspace holds. The records
// carry ciphertext only, and this drops even that.
func (a *credentialAPI) listCredentials(w http.ResponseWriter, r *http.Request) {
	records, err := a.credentials.List(r.Context(), chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	views := make([]credentialView, 0, len(records))
	for _, record := range records {
		views = append(views, credentialView{
			Provider:  record.Provider,
			Name:      record.Name,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": views})
}

func (a *credentialAPI) deleteCredential(w http.ResponseWriter, r *http.Request) {
	provider, name, err := credentialPath(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := a.credentials.Delete(r.Context(), chi.URLParam(r, "workspaceID"), provider, name); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// credentialPath pulls the provider and name out of the URL and rejects the
// shapes the service would refuse anyway, so a bad request is a 400 here
// rather than a 500 further down.
func credentialPath(r *http.Request) (provider, name string, err error) {
	provider = strings.TrimSpace(chi.URLParam(r, "provider"))
	name = strings.TrimSpace(chi.URLParam(r, "name"))

	if provider == "" || name == "" {
		return "", "", invalid("provider and name are required")
	}
	if len(provider) > 64 || len(name) > 64 {
		return "", "", invalid("provider and name are limited to 64 characters")
	}
	return provider, name, nil
}

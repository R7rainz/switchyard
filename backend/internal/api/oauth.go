package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/oauth"
)

type oauthAPI struct {
	service     *oauth.Service
	callbackURL string
	appURL      string
}

func (a *oauthAPI) start(w http.ResponseWriter, r *http.Request) {
	if a.service == nil {
		http.Error(w, "OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider := chi.URLParam(r, "provider")
	callback := strings.TrimSuffix(a.callbackURL, "/") + "/" + url.PathEscape(provider)
	authorize, err := a.service.Start(r.Context(), chi.URLParam(r, "workspaceID"), provider, callback)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": authorize})
}

func (a *oauthAPI) callback(w http.ResponseWriter, r *http.Request) {
	if a.service == nil {
		http.Error(w, "OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider := chi.URLParam(r, "provider")
	callback := strings.TrimSuffix(a.callbackURL, "/") + "/" + url.PathEscape(provider)
	workspaceID, err := a.service.Callback(r.Context(), provider, r.URL.Query().Get("code"), r.URL.Query().Get("state"), callback)
	if err != nil {
		// Do not reflect provider/token errors into a browser URL. The caller gets
		// a stable failure and the server log carries the request id.
		zerolog.Ctx(r.Context()).Warn().Err(err).Msg("oauth callback failed")
		http.Redirect(w, r, a.appURL+"/settings?oauth=error", http.StatusFound)
		return
	}
	http.Redirect(w, r, a.appURL+"/settings?oauth=connected&provider="+url.QueryEscape(provider)+"&workspace="+url.QueryEscape(workspaceID), http.StatusFound)
}

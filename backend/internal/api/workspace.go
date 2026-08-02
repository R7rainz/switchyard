package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// Authorizer is the part of the workspace service this layer needs: given a
// workspace, a caller, and a permission, say yes or why not.
type Authorizer interface {
	Authorize(ctx context.Context, workspaceID, userID string, p auth.Permission) (auth.Role, error)
}

// workspaceContextKey carries the caller's role for the handlers below.
type workspaceContextKey struct{}

// WorkspaceRole returns the role the caller holds in the workspace the URL
// names. It is only present below RequirePermission.
func WorkspaceRole(ctx context.Context) (auth.Role, bool) {
	role, ok := ctx.Value(workspaceContextKey{}).(auth.Role)
	return role, ok
}

// RequirePermission gates a route on the caller holding p in the workspace
// named by the {workspaceID} URL parameter.
//
// The check runs per request rather than off the token: roles change, and a
// token minted before a demotion must not keep the access it was minted with.
func RequirePermission(authorizer Authorizer, p auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := auth.UserID(r.Context())
			if err != nil {
				writeError(w, r, err)
				return
			}

			workspaceID := chi.URLParam(r, "workspaceID")
			if workspaceID == "" {
				// A route mounted without the parameter would otherwise check
				// permission against the empty workspace and pass nobody, or
				// worse, be read as global.
				writeError(w, r, errNoWorkspace)
				return
			}

			role, err := authorizer.Authorize(r.Context(), workspaceID, userID, p)
			if err != nil {
				writeError(w, r, err)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), workspaceContextKey{}, role)))
		})
	}
}

// workspaceAPI carries what the workspace routes call, so the service is
// threaded once instead of through nine closures.
type workspaceAPI struct {
	workspaces *workspace.Service
	appURL     string
}

const (
	// maxNameLen is long enough for any real team name and short enough that
	// nothing downstream has to think about the field.
	maxNameLen = 80
	maxSlugLen = 64
)

// listWorkspaces returns the caller's workspaces, and gives an account that has
// none its first one.
//
// Better Auth signup creates a user and nothing else, so a new account belongs
// to no workspace and every workspace-scoped call would 404 forever. The
// bootstrap lives here because this route is where it is free: the listing
// query is the same query the check would need, and this is the one call a
// client cannot skip, since it has no workspace id to use anywhere else until
// it has made it. In RequireAuth the check would cost a query on every request
// for the life of the account to serve something that happens once, and as its
// own endpoint it would be one the frontend can forget to call.
func (a *workspaceAPI) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok || claims.Subject == "" {
		writeError(w, r, auth.ErrNoIdentity)
		return
	}

	found, err := a.workspaces.List(r.Context(), claims.Subject)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if len(found) == 0 {
		created, err := a.workspaces.Create(r.Context(), claims.Subject, personalName(claims), personalSlug(claims.Subject))
		if err != nil {
			writeError(w, r, err)
			return
		}
		found = []workspace.Workspace{created}
	}

	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspacesJSON(found)})
}

func (a *workspaceAPI) createWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, r, invalid("name is required"))
		return
	}
	if len(name) > maxNameLen {
		writeError(w, r, invalid("name is longer than %d characters", maxNameLen))
		return
	}

	// A slug the client did not send is derived, since the name is the only
	// thing a user actually wants to choose.
	slug := slugify(body.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	if slug == "" {
		writeError(w, r, invalid("slug must contain a letter or a digit"))
		return
	}
	if len(slug) > maxSlugLen {
		writeError(w, r, invalid("slug is longer than %d characters", maxSlugLen))
		return
	}

	created, err := a.workspaces.Create(r.Context(), userID, name, slug)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, workspaceJSON(created))
}

func (a *workspaceAPI) listMembers(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	members, err := a.workspaces.Members(r.Context(), chi.URLParam(r, "workspaceID"), userID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	out := make([]map[string]any, 0, len(members))
	for _, member := range members {
		out = append(out, map[string]any{
			"userId":    member.UserID,
			"role":      string(member.Role),
			"createdAt": member.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (a *workspaceAPI) setRole(w http.ResponseWriter, r *http.Request) {
	callerID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	role := auth.Role(body.Role)
	if !role.Valid() {
		// Caught here rather than in the service so an unknown role reads as a
		// bad request instead of an internal failure.
		writeError(w, r, invalid("%q is not a role", body.Role))
		return
	}

	if err := a.workspaces.SetRole(r.Context(), chi.URLParam(r, "workspaceID"), callerID, chi.URLParam(r, "userID"), role); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"userId": chi.URLParam(r, "userID"), "role": string(role)})
}

func (a *workspaceAPI) removeMember(w http.ResponseWriter, r *http.Request) {
	callerID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := a.workspaces.Remove(r.Context(), chi.URLParam(r, "workspaceID"), callerID, chi.URLParam(r, "userID")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *workspaceAPI) createInvite(w http.ResponseWriter, r *http.Request) {
	callerID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
		// TTLHours overrides the seven-day default; negative means no expiry.
		TTLHours float64 `json:"ttlHours"`
		MaxUses  int     `json:"maxUses"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	role := auth.Role(body.Role)
	if !role.Valid() {
		writeError(w, r, invalid("%q is not a role", body.Role))
		return
	}
	email := strings.TrimSpace(body.Email)
	if email != "" && !strings.Contains(email, "@") {
		writeError(w, r, invalid("email is not an address"))
		return
	}
	if body.MaxUses < 0 {
		writeError(w, r, invalid("maxUses cannot be negative"))
		return
	}

	created, token, err := a.workspaces.Invite(r.Context(), chi.URLParam(r, "workspaceID"), callerID, workspace.InviteRequest{
		Email:   email,
		Role:    role,
		TTL:     time.Duration(body.TTLHours * float64(time.Hour)),
		MaxUses: body.MaxUses,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	// The only moment this token exists outside the invitee's hands: only its
	// hash is stored, so it cannot be shown again. Re-sharing means revoke and
	// re-issue.
	writeJSON(w, http.StatusCreated, map[string]any{
		"invite":  inviteJSON(created),
		"token":   token,
		"joinURL": a.joinURL(token),
	})
}

func (a *workspaceAPI) listInvites(w http.ResponseWriter, r *http.Request) {
	callerID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	invites, err := a.workspaces.Invites(r.Context(), chi.URLParam(r, "workspaceID"), callerID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	out := make([]map[string]any, 0, len(invites))
	for _, invite := range invites {
		out = append(out, inviteJSON(invite))
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (a *workspaceAPI) revokeInvite(w http.ResponseWriter, r *http.Request) {
	callerID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := a.workspaces.Revoke(r.Context(), chi.URLParam(r, "workspaceID"), callerID, chi.URLParam(r, "inviteID")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *workspaceAPI) acceptInvite(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserID(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	joined, err := a.workspaces.Accept(r.Context(), chi.URLParam(r, "token"), userID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceJSON(joined))
}

// joinURL points at the frontend rather than at this API: the invitee needs a
// page that can sign them in first, since accepting requires an identity.
func (a *workspaceAPI) joinURL(token string) string {
	return strings.TrimSuffix(a.appURL, "/") + "/invite/" + token
}

func workspaceJSON(w workspace.Workspace) map[string]any {
	return map[string]any{"id": w.ID, "name": w.Name, "slug": w.Slug, "createdAt": w.CreatedAt}
}

func workspacesJSON(found []workspace.Workspace) []map[string]any {
	out := make([]map[string]any, 0, len(found))
	for _, one := range found {
		out = append(out, workspaceJSON(one))
	}
	return out
}

// inviteJSON deliberately omits TokenHash. The hash is not the credential, but
// publishing it would let someone confirm a guessed token offline, and no
// client has any use for it.
func inviteJSON(i workspace.Invite) map[string]any {
	out := map[string]any{
		"id":        i.ID,
		"role":      string(i.Role),
		"email":     i.Email,
		"link":      i.IsLink(),
		"maxUses":   i.MaxUses,
		"useCount":  i.UseCount,
		"invitedBy": i.InvitedBy,
		"createdAt": i.CreatedAt,
	}
	if !i.ExpiresAt.IsZero() {
		out["expiresAt"] = i.ExpiresAt
	}
	return out
}

// personalName labels the workspace an account gets on its first request. The
// claims are whatever Better Auth had, so every field can be empty.
func personalName(claims *auth.Claims) string {
	who := strings.TrimSpace(claims.Name)
	if who == "" {
		who, _, _ = strings.Cut(claims.Email, "@")
	}
	if who == "" {
		return "Personal workspace"
	}
	if len(who) > maxNameLen-12 {
		who = who[:maxNameLen-12]
	}
	return who + "'s workspace"
}

// personalSlug is built from the user id because the slug is unique in the
// schema and the id is the only value at hand guaranteed not to collide.
func personalSlug(userID string) string {
	return "u-" + slugify(userID)
}

// slugify reduces a name to lowercase letters, digits, and single dashes.
// Anything else becomes a separator, so the result is safe in a URL.
func slugify(name string) string {
	var out strings.Builder
	dashed := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out.WriteRune(r)
			dashed = false
		case !dashed && out.Len() > 0:
			out.WriteByte('-')
			dashed = true
		}
	}
	return strings.Trim(out.String(), "-")
}

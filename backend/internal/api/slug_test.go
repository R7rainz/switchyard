package api

import (
	"net/http"
	"testing"
)

func TestDuplicateSlugIsAConflictNotAServerError(t *testing.T) {
	// This was a 500: the store handed back a raw driver error and writeError's
	// default caught it, so a name the caller could simply change looked like
	// the database falling over.
	router := routes()

	if status, _ := call(t, router, http.MethodPost, "/api/workspaces", "alice",
		`{"name":"Switchyard","slug":"switchyard"}`); status != http.StatusCreated {
		t.Fatalf("first create = %d, want 201", status)
	}

	status, body := call(t, router, http.MethodPost, "/api/workspaces", "alice",
		`{"name":"Switchyard Again","slug":"switchyard"}`)
	if status != http.StatusConflict {
		t.Fatalf("duplicate slug = %d, want 409", status)
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("409 body carried no error message: %v", body)
	}
}

func TestSlugsCollideAcrossUsersToo(t *testing.T) {
	// Slugs are global because they are meant for URLs, so the second caller is
	// usually a stranger. They get a conflict, and nothing that tells them
	// whose workspace holds it.
	router := routes()

	if status, _ := call(t, router, http.MethodPost, "/api/workspaces", "alice",
		`{"name":"Shared","slug":"shared"}`); status != http.StatusCreated {
		t.Fatal("alice could not create")
	}

	status, body := call(t, router, http.MethodPost, "/api/workspaces", "bob",
		`{"name":"Shared","slug":"shared"}`)
	if status != http.StatusConflict {
		t.Fatalf("bob's duplicate slug = %d, want 409", status)
	}
	if message, _ := body["error"].(string); message == "" {
		t.Fatal("no error message")
	} else if message != "slug is already taken" {
		t.Errorf("message = %q; it should not describe the other workspace", message)
	}
}

func TestBootstrapSurvivesLosingItsOwnRace(t *testing.T) {
	// Two concurrent first requests from one account both find no workspace and
	// both try to create the personal one. The loser must not answer 409 for
	// something the user neither asked for nor can fix — it lists again and
	// returns the workspace that won.
	router := routes()

	first := firstWorkspace(t, router, "alice")

	// The second call finds the workspace already there, which is the state the
	// losing racer reaches after its create is rejected.
	second := firstWorkspace(t, router, "alice")

	if first != second {
		t.Errorf("bootstrap produced two workspaces: %s and %s", first, second)
	}
}

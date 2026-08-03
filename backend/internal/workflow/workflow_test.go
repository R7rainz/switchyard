package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// wsA and wsB are two workspaces, because a rule worth proving repeatedly is
// that a workflow in one is invisible from the other.
const (
	wsA  = "ws-a"
	wsB  = "ws-b"
	user = "user-1"
)

func service(t *testing.T) *Service {
	t.Helper()
	return NewService(NewMemoryStore())
}

func mustCreate(t *testing.T, s *Service) Workflow {
	t.Helper()
	created, err := s.Create(context.Background(), wsA, user, "deploy on merge", "", validGraph())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	s := service(t)

	created, err := s.Create(ctx, wsA, user, "  deploy on merge  ", "ships main", validGraph())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned no id")
	}
	if created.Name != "deploy on merge" {
		t.Fatalf("name = %q, want it trimmed", created.Name)
	}
	if created.WorkspaceID != wsA || created.CreatedBy != user {
		t.Fatalf("ownership not recorded: %+v", created)
	}
	if created.CreatedAt.IsZero() || !created.UpdatedAt.Equal(created.CreatedAt) {
		t.Fatalf("timestamps wrong: %+v", created)
	}
}

// A corrupt graph never reaches the store, by any path that writes one.
func TestCreateRejectsCorruptGraph(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	s := NewService(store)

	dangling := Graph{
		Nodes: []Node{node("t", "trigger.manual")},
		Edges: []Edge{edge("e1", "t", "ghost")},
	}
	if _, err := s.Create(ctx, wsA, user, "bad", "", dangling); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}

	stored, err := store.List(ctx, wsA)
	if err != nil || len(stored) != 0 {
		t.Fatalf("a rejected graph was stored anyway: %v, %v", stored, err)
	}
}

// The other half of the split: a workflow nobody could run yet still saves,
// because that is what a builder canvas looks like most of the time.
func TestCreateAcceptsAnUnfinishedDraft(t *testing.T) {
	ctx := context.Background()
	s := service(t)

	draft := Graph{Nodes: []Node{node("floating", "http.request")}}
	created, err := s.Create(ctx, wsA, user, "work in progress", "", draft)
	if err != nil {
		t.Fatalf("a draft must save: %v", err)
	}
	if err := created.Graph.Runnable(); err == nil {
		t.Fatal("this draft should not be runnable, so the test proves nothing")
	}
}

func TestCreateRejectsBadNames(t *testing.T) {
	ctx := context.Background()
	s := service(t)

	cases := map[string]string{
		"blank":    "   ",
		"empty":    "",
		"too long": strings.Repeat("x", maxNameLen+1),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Create(ctx, wsA, user, value, "", validGraph()); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want ErrInvalid", err)
			}
		})
	}

	long := strings.Repeat("x", maxDescriptionLen+1)
	if _, err := s.Create(ctx, wsA, user, "fine", long, validGraph()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long description: got %v, want ErrInvalid", err)
	}
}

// A patch touches what it names and nothing else. Getting this wrong means a
// rename silently blanks a description, which is the kind of data loss nobody
// notices until the description mattered.
func TestUpdateIsPartial(t *testing.T) {
	ctx := context.Background()
	s := service(t)

	created, err := s.Create(ctx, wsA, user, "original", "keep me", validGraph())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	renamed := "renamed"
	updated, err := s.Update(ctx, wsA, created.ID, Patch{Name: &renamed})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("name = %q, want %q", updated.Name, "renamed")
	}
	if updated.Description != "keep me" {
		t.Fatalf("description = %q, want it untouched", updated.Description)
	}
	if len(updated.Graph.Nodes) != len(created.Graph.Nodes) {
		t.Fatal("graph changed on a rename")
	}

	// An explicit empty string is a real value, not an omission.
	blank := ""
	updated, err = s.Update(ctx, wsA, created.ID, Patch{Description: &blank})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Description != "" {
		t.Fatalf("description = %q, want it cleared", updated.Description)
	}
	if updated.Name != "renamed" {
		t.Fatalf("name = %q, want it untouched", updated.Name)
	}
}

func TestUpdateRejectsCorruptGraph(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	s := NewService(store)
	created := mustCreate(t, s)

	duplicate := Graph{
		Nodes: []Node{node("t", "trigger.manual"), node("t", "http.request")},
		Edges: []Edge{edge("e1", "t", "t")},
	}
	if _, err := s.Update(ctx, wsA, created.ID, Patch{Graph: &duplicate}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}

	// The stored graph is the one that was valid.
	stored, err := store.Get(ctx, wsA, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(stored.Graph.Edges) != 1 {
		t.Fatalf("the rejected graph was stored anyway: %+v", stored.Graph)
	}
}

// Permission was checked against the workspace in the URL, so the workspace
// has to reach the query or a valid caller reads someone else's workflow.
func TestUpdateAndDeleteAreWorkspaceScoped(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	created := mustCreate(t, s)

	name := "stolen"
	if _, err := s.Update(ctx, wsB, created.ID, Patch{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update: got %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, wsB, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete: got %v, want ErrNotFound", err)
	}
	if _, err := s.Get(ctx, wsB, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get: got %v, want ErrNotFound", err)
	}
}

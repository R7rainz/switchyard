package workflow

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"
)

// ErrNotFound means no workflow with that id exists in that workspace. A
// workflow belonging to another workspace is not found either — the workspace
// is part of the identity, not a filter that can be omitted.
var ErrNotFound = errors.New("workflow: not found")

// Workflow is a saved graph and the handful of things a dashboard needs to
// list it. It describes what should happen; running it belongs to the
// execution package.
type Workflow struct {
	ID          string
	WorkspaceID string
	Name        string
	Description string
	Graph       Graph
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Store is the persistence this package needs, declared here where it is
// consumed so the rules can be tested without a database.
//
// Every method takes a workspace id and must put it in the WHERE clause.
// A workflow id alone is enough to name a row, which is exactly the problem:
// the API layer checks permission against the workspace in the URL, so a store
// that looked a workflow up by id alone would hand workspace B's admin a
// workflow from workspace A. Making the workspace a required argument means
// that leak takes a deliberately wrong query rather than a forgotten filter.
//
// This package scopes; it does not authorize. Whether the caller may touch the
// workspace at all is settled earlier, by api.RequirePermission.
type Store interface {
	Create(ctx context.Context, w Workflow) error
	Get(ctx context.Context, workspaceID, id string) (Workflow, error)

	// List returns every workflow in the workspace, graphs included. Loading
	// graphs the dashboard does not draw is waste, but a Workflow whose Graph
	// was silently left empty is a trap; if listing ever gets slow, add a
	// separate summary type rather than a half-populated one.
	List(ctx context.Context, workspaceID string) ([]Workflow, error)

	Update(ctx context.Context, w Workflow) error
	Delete(ctx context.Context, workspaceID, id string) error
}

// Service holds the rules about what a workflow may look like. There is really
// one rule — the graph must validate — and it is applied on every path that
// writes a graph, so an invalid graph cannot reach the database by any route.
type Service struct {
	store Store
	now   func() time.Time
	newID func() string
}

// NewService returns a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now, newID: randomID}
}

const maxNameLen = 200
const maxDescriptionLen = 2000

// Create validates the graph and stores it.
func (s *Service) Create(ctx context.Context, workspaceID, creatorID, name, description string, graph Graph) (Workflow, error) {
	if workspaceID == "" {
		return Workflow{}, errors.New("workflow: workspace is required")
	}
	name, err := checkName(name)
	if err != nil {
		return Workflow{}, err
	}
	if err := checkDescription(description); err != nil {
		return Workflow{}, err
	}
	if err := graph.Validate(); err != nil {
		return Workflow{}, err
	}

	now := s.now()
	created := Workflow{
		ID:          s.newID(),
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		Graph:       graph,
		CreatedBy:   creatorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.Create(ctx, created); err != nil {
		return Workflow{}, err
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, workspaceID, id string) (Workflow, error) {
	return s.store.Get(ctx, workspaceID, id)
}

func (s *Service) List(ctx context.Context, workspaceID string) ([]Workflow, error) {
	return s.store.List(ctx, workspaceID)
}

func (s *Service) Delete(ctx context.Context, workspaceID, id string) error {
	return s.store.Delete(ctx, workspaceID, id)
}

// Patch is a partial update. The fields are pointers so that "not sent" and
// "set to empty" stay different things: a request that only renames a workflow
// must not blank its description, and one that only edits the graph must not
// rename it.
type Patch struct {
	Name        *string
	Description *string
	Graph       *Graph
}

// Update applies a patch and returns the workflow as stored.
//
// Read, modify, write, with no locking: two people editing one workflow at the
// same time means the later save wins outright. That is the honest behaviour
// for a single-editor builder, and the fix when it stops being true is a
// revision column the client echoes back, not a transaction here.
func (s *Service) Update(ctx context.Context, workspaceID, id string, patch Patch) (Workflow, error) {
	stored, err := s.store.Get(ctx, workspaceID, id)
	if err != nil {
		return Workflow{}, err
	}

	if patch.Name != nil {
		name, err := checkName(*patch.Name)
		if err != nil {
			return Workflow{}, err
		}
		stored.Name = name
	}
	if patch.Description != nil {
		if err := checkDescription(*patch.Description); err != nil {
			return Workflow{}, err
		}
		stored.Description = *patch.Description
	}
	if patch.Graph != nil {
		// Validated on the way in, every time, rather than trusting that
		// whatever is already stored was fine and this edit is small.
		if err := patch.Graph.Validate(); err != nil {
			return Workflow{}, err
		}
		stored.Graph = *patch.Graph
	}

	stored.UpdatedAt = s.now()
	if err := s.store.Update(ctx, stored); err != nil {
		return Workflow{}, err
	}
	return stored, nil
}

func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("a workflow needs a name")
	}
	if len(name) > maxNameLen {
		return "", invalid("name is limited to %d characters", maxNameLen)
	}
	return name, nil
}

func checkDescription(description string) error {
	if len(description) > maxDescriptionLen {
		return invalid("description is limited to %d characters", maxDescriptionLen)
	}
	return nil
}

// randomID is 26 base32 characters from crypto/rand — collision-proof in
// practice and safe to put in a URL.
func randomID() string { return rand.Text() }

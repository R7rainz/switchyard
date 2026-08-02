package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

// PostgresStore is the real Store. The queries live here, next to the rules
// they serve, rather than in the database package — that one owns the pool and
// the schema and nothing else.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a Store backed by pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateWorkspace(ctx context.Context, w Workspace) error {
	_, err := s.pool.Exec(ctx,
		`insert into "workspace" ("id", "name", "slug", "createdAt", "updatedAt")
		 values ($1, $2, $3, $4, $4)`,
		w.ID, w.Name, w.Slug, w.CreatedAt)
	if err != nil {
		return fmt.Errorf("workspace: creating: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetWorkspace(ctx context.Context, id string) (Workspace, error) {
	var w Workspace
	err := s.pool.QueryRow(ctx,
		`select "id", "name", "slug", "createdAt" from "workspace" where "id" = $1`,
		id).Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace: loading: %w", err)
	}
	return w, nil
}

// ListWorkspacesForUser joins through membership so the predicate is in the
// query. A version that listed everything and filtered afterwards would return
// other people's workspaces the moment someone forgot the filter.
func (s *PostgresStore) ListWorkspacesForUser(ctx context.Context, userID string) ([]Workspace, error) {
	rows, err := s.pool.Query(ctx,
		`select w."id", w."name", w."slug", w."createdAt"
		 from "workspace" w
		 join "workspace_member" m on m."workspaceId" = w."id"
		 where m."userId" = $1
		 order by w."createdAt"`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("workspace: listing for user: %w", err)
	}
	defer rows.Close()

	var found []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("workspace: listing for user: %w", err)
		}
		found = append(found, w)
	}
	return found, rows.Err()
}

// PutMember inserts a membership or updates the role of the one already there.
//
// Member carries no id of its own — the pair of workspace and user is what
// identifies it — so the generated id below is only used when this turns out
// to be an insert. On conflict the existing row keeps its id and createdAt,
// which is what makes a role change a change rather than a rejoin.
func (s *PostgresStore) PutMember(ctx context.Context, m Member) error {
	_, err := s.pool.Exec(ctx,
		`insert into "workspace_member" ("id", "workspaceId", "userId", "role", "createdAt")
		 values ($1, $2, $3, $4, $5)
		 on conflict ("workspaceId", "userId") do update set "role" = excluded."role"`,
		rand.Text(), m.WorkspaceID, m.UserID, string(m.Role), m.CreatedAt)
	if err != nil {
		return fmt.Errorf("workspace: saving member: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetMember(ctx context.Context, workspaceID, userID string) (Member, error) {
	var m Member
	var role string
	err := s.pool.QueryRow(ctx,
		`select "workspaceId", "userId", "role", "createdAt"
		 from "workspace_member" where "workspaceId" = $1 and "userId" = $2`,
		workspaceID, userID).Scan(&m.WorkspaceID, &m.UserID, &role, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("workspace: loading member: %w", err)
	}
	m.Role = auth.Role(role)
	return m, nil
}

func (s *PostgresStore) ListMembers(ctx context.Context, workspaceID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`select "workspaceId", "userId", "role", "createdAt"
		 from "workspace_member" where "workspaceId" = $1 order by "createdAt"`,
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace: listing members: %w", err)
	}
	defer rows.Close()

	var found []Member
	for rows.Next() {
		var m Member
		var role string
		if err := rows.Scan(&m.WorkspaceID, &m.UserID, &role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("workspace: listing members: %w", err)
		}
		m.Role = auth.Role(role)
		found = append(found, m)
	}
	return found, rows.Err()
}

func (s *PostgresStore) DeleteMember(ctx context.Context, workspaceID, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`delete from "workspace_member" where "workspaceId" = $1 and "userId" = $2`,
		workspaceID, userID)
	if err != nil {
		return fmt.Errorf("workspace: removing member: %w", err)
	}
	// Deleting nothing is reported rather than swallowed: the caller asked to
	// remove a specific person, and silence would look like success.
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) CreateInvite(ctx context.Context, i Invite) error {
	_, err := s.pool.Exec(ctx,
		`insert into "workspace_invite"
		   ("id", "workspaceId", "tokenHash", "email", "role", "expiresAt", "maxUses", "useCount", "invitedBy", "createdAt")
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		i.ID, i.WorkspaceID, i.TokenHash, nullString(i.Email), string(i.Role),
		nullTime(i.ExpiresAt), nullInt(i.MaxUses), i.UseCount, nullString(i.InvitedBy), i.CreatedAt)
	if err != nil {
		return fmt.Errorf("workspace: creating invite: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetInviteByTokenHash(ctx context.Context, tokenHash string) (Invite, error) {
	row := s.pool.QueryRow(ctx, inviteColumns+` where "tokenHash" = $1`, tokenHash)
	invite, err := scanInvite(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, fmt.Errorf("workspace: loading invite: %w", err)
	}
	return invite, nil
}

func (s *PostgresStore) ListInvites(ctx context.Context, workspaceID string) ([]Invite, error) {
	rows, err := s.pool.Query(ctx, inviteColumns+` where "workspaceId" = $1 order by "createdAt" desc`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace: listing invites: %w", err)
	}
	defer rows.Close()

	var found []Invite
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("workspace: listing invites: %w", err)
		}
		found = append(found, invite)
	}
	return found, rows.Err()
}

// UpdateInvite writes back the mutable part of an invite, which in practice is
// only the use count. The token hash is never rewritten.
func (s *PostgresStore) UpdateInvite(ctx context.Context, i Invite) error {
	tag, err := s.pool.Exec(ctx,
		`update "workspace_invite"
		 set "role" = $2, "expiresAt" = $3, "maxUses" = $4, "useCount" = $5
		 where "id" = $1`,
		i.ID, string(i.Role), nullTime(i.ExpiresAt), nullInt(i.MaxUses), i.UseCount)
	if err != nil {
		return fmt.Errorf("workspace: updating invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteInvite(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `delete from "workspace_invite" where "id" = $1`, id)
	if err != nil {
		return fmt.Errorf("workspace: revoking invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const inviteColumns = `select "id", "workspaceId", "tokenHash", "email", "role",
	"expiresAt", "maxUses", "useCount", "invitedBy", "createdAt" from "workspace_invite"`

// scanInvite reads one row, turning the nullable columns back into the zero
// values the domain uses: no email means a shareable link, no expiry means it
// never expires, no cap means unlimited uses.
func scanInvite(row pgx.Row) (Invite, error) {
	var (
		invite    Invite
		role      string
		email     *string
		expiresAt *time.Time
		maxUses   *int
		invitedBy *string
	)

	if err := row.Scan(&invite.ID, &invite.WorkspaceID, &invite.TokenHash, &email, &role,
		&expiresAt, &maxUses, &invite.UseCount, &invitedBy, &invite.CreatedAt); err != nil {
		return Invite{}, err
	}

	invite.Role = auth.Role(role)
	if email != nil {
		invite.Email = *email
	}
	if expiresAt != nil {
		invite.ExpiresAt = *expiresAt
	}
	if maxUses != nil {
		invite.MaxUses = *maxUses
	}
	if invitedBy != nil {
		invite.InvitedBy = *invitedBy
	}
	return invite, nil
}

// The domain uses zero values where the schema uses NULL, so that "unlimited"
// and "never" are one concept each rather than two.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nullInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// PostgresStore and MemoryStore must stay interchangeable; the tests run the
// same expectations against both.
var _ Store = (*PostgresStore)(nil)

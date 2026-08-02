package auth

// Role is a member's standing inside one workspace. The four are strictly
// ordered: every role can do everything the role below it can.
type Role string

const (
	RoleViewer Role = "VIEWER"
	RoleMember Role = "MEMBER"
	RoleAdmin  Role = "ADMIN"
	RoleOwner  Role = "OWNER"
)

// rank orders the roles. Anything unrecognised ranks below VIEWER so a corrupt
// or hand-edited row grants nothing rather than everything.
var rank = map[Role]int{
	RoleViewer: 1,
	RoleMember: 2,
	RoleAdmin:  3,
	RoleOwner:  4,
}

// Valid reports whether r is one of the four known roles.
func (r Role) Valid() bool {
	_, ok := rank[r]
	return ok
}

// AtLeast reports whether r carries at least the standing of other.
func (r Role) AtLeast(other Role) bool {
	mine, ok := rank[r]
	if !ok {
		return false
	}
	theirs, ok := rank[other]
	if !ok {
		return false
	}
	return mine >= theirs
}

// Permission is a single thing a member may attempt in a workspace.
type Permission string

const (
	PermissionWorkflowRead   Permission = "workflow:read"
	PermissionWorkflowWrite  Permission = "workflow:write"
	PermissionWorkflowDelete Permission = "workflow:delete"

	PermissionExecutionRead Permission = "execution:read"
	PermissionExecutionRun  Permission = "execution:run"

	// Credentials are write-only from the outside: they can be set and
	// replaced, never read back in plaintext, so there is no read permission
	// to grant.
	PermissionCredentialManage Permission = "credential:manage"

	PermissionMemberRead   Permission = "member:read"
	PermissionMemberManage Permission = "member:manage"

	PermissionWorkspaceUpdate Permission = "workspace:update"
	PermissionWorkspaceDelete Permission = "workspace:delete"
)

// minimumRole is the whole authorization table. One place to read, one place
// to change.
//
// Credentials sit at ADMIN rather than MEMBER: a member who can run workflows
// already uses the stored keys, but only an admin decides which keys exist.
var minimumRole = map[Permission]Role{
	PermissionWorkflowRead:   RoleViewer,
	PermissionWorkflowWrite:  RoleMember,
	PermissionWorkflowDelete: RoleMember,

	PermissionExecutionRead: RoleViewer,
	PermissionExecutionRun:  RoleMember,

	PermissionCredentialManage: RoleAdmin,

	PermissionMemberRead:   RoleViewer,
	PermissionMemberManage: RoleAdmin,

	PermissionWorkspaceUpdate: RoleAdmin,
	PermissionWorkspaceDelete: RoleOwner,
}

// Can reports whether this role may do p.
//
// An unknown permission is denied. Defaulting to the zero Role would rank it
// at nothing and let everyone through, so a typo in a call site would open a
// hole rather than close one.
func (r Role) Can(p Permission) bool {
	required, ok := minimumRole[p]
	if !ok {
		return false
	}
	return r.AtLeast(required)
}

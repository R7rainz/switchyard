package auth

import "testing"

func TestRoleHierarchy(t *testing.T) {
	ordered := []Role{RoleViewer, RoleMember, RoleAdmin, RoleOwner}

	for i, lower := range ordered {
		for j, higher := range ordered {
			want := j >= i
			if got := higher.AtLeast(lower); got != want {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", higher, lower, got, want)
			}
		}
	}
}

func TestUnknownRoleGrantsNothing(t *testing.T) {
	// A corrupt or hand-edited row must rank below everything, not above it.
	for _, role := range []Role{"", "SUPERUSER", "owner", "root"} {
		t.Run(string(role), func(t *testing.T) {
			if role.Valid() {
				t.Errorf("%q reported valid", role)
			}
			if role.AtLeast(RoleViewer) {
				t.Errorf("%q outranked VIEWER", role)
			}
			if role.Can(PermissionWorkflowRead) {
				t.Errorf("%q was granted the lowest permission there is", role)
			}
		})
	}
}

func TestUnknownPermissionIsDenied(t *testing.T) {
	// A typo at a call site must close a door, not open one. An owner is the
	// strongest case: if even OWNER is refused, nobody gets through.
	if RoleOwner.Can(Permission("workflow:destroy")) {
		t.Error("OWNER was granted an unknown permission")
	}
	if RoleOwner.Can("") {
		t.Error("OWNER was granted the empty permission")
	}
}

func TestPermissionMatrix(t *testing.T) {
	tests := []struct {
		role       Role
		permission Permission
		want       bool
	}{
		{RoleViewer, PermissionWorkflowRead, true},
		{RoleViewer, PermissionExecutionRead, true},
		{RoleViewer, PermissionWorkflowWrite, false},
		{RoleViewer, PermissionExecutionRun, false},

		{RoleMember, PermissionWorkflowWrite, true},
		{RoleMember, PermissionWorkflowDelete, true},
		{RoleMember, PermissionExecutionRun, true},
		// A member runs workflows that use the stored keys, but does not get
		// to decide which keys exist.
		{RoleMember, PermissionCredentialManage, false},
		{RoleMember, PermissionMemberManage, false},

		{RoleAdmin, PermissionCredentialManage, true},
		{RoleAdmin, PermissionMemberManage, true},
		{RoleAdmin, PermissionWorkspaceUpdate, true},
		{RoleAdmin, PermissionWorkspaceDelete, false},

		{RoleOwner, PermissionWorkspaceDelete, true},
	}

	for _, tc := range tests {
		if got := tc.role.Can(tc.permission); got != tc.want {
			t.Errorf("%s.Can(%s) = %v, want %v", tc.role, tc.permission, got, tc.want)
		}
	}
}

func TestEveryPermissionIsInTheMatrix(t *testing.T) {
	// A permission constant with no entry would be denied to everyone, which
	// is safe but silently breaks the feature that uses it.
	all := []Permission{
		PermissionWorkflowRead, PermissionWorkflowWrite, PermissionWorkflowDelete,
		PermissionExecutionRead, PermissionExecutionRun,
		PermissionCredentialManage,
		PermissionMemberRead, PermissionMemberManage,
		PermissionWorkspaceUpdate, PermissionWorkspaceDelete,
	}
	for _, p := range all {
		if _, ok := minimumRole[p]; !ok {
			t.Errorf("permission %q has no entry in minimumRole", p)
		}
	}
	if len(all) != len(minimumRole) {
		t.Errorf("matrix has %d entries but %d permissions are declared", len(minimumRole), len(all))
	}
}

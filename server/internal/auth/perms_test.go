package auth

import "testing"

func TestRoleHasPerm(t *testing.T) {
	cases := []struct {
		role, perm string
		want       bool
	}{
		{RoleAdmin, PermAlertPublish, true},
		{RoleAdmin, PermSAManage, true},
		{"owner", PermOrgManage, true},
		{"dispatcher", PermAlertCancel, true},
		{"dispatcher", PermSAManage, false},
		{RoleOperator, PermDeviceProvision, true},
		{RoleOperator, PermAlertCancel, false},
		{"viewer", PermAlertRead, true},
		{"viewer", PermAlertPublish, false},
		{RoleUser, PermAlertRead, true},
		{RoleUser, PermSAManage, false},
		{"nonexistent", PermAlertRead, false},
	}
	for _, c := range cases {
		if got := RoleHasPerm(c.role, c.perm); got != c.want {
			t.Errorf("RoleHasPerm(%q,%q)=%v want %v", c.role, c.perm, got, c.want)
		}
	}
}

package auth

// Permission catalog (resource:action). RBAC: a role maps to a set of permissions
// (Grafana-style). Membership role within an org decides what a user may do.
const (
	PermAlertPublish    = "alert:publish"
	PermAlertCancel     = "alert:cancel"
	PermAlertRead       = "alert:read"
	PermDeviceRead      = "device:read"
	PermDeviceProvision = "device:provision"
	PermSAManage        = "sa:manage"
	PermMemberManage    = "member:manage"
	PermOrgManage       = "org:manage"
	PermSettingsManage  = "settings:manage"
)

func set(perms ...string) map[string]bool {
	m := make(map[string]bool, len(perms))
	for _, p := range perms {
		m[p] = true
	}
	return m
}

func all() map[string]bool {
	return set(PermAlertPublish, PermAlertCancel, PermAlertRead, PermDeviceRead,
		PermDeviceProvision, PermSAManage, PermMemberManage, PermOrgManage, PermSettingsManage)
}

// rolePerms maps base roles → permission sets. Includes the current roles
// (admin/operator/user) plus the richer enterprise roles.
var rolePerms = map[string]map[string]bool{
	RoleAdmin:    all(),
	"owner":      all(),
	"org_admin":  all(),
	"dispatcher": set(PermAlertPublish, PermAlertCancel, PermAlertRead, PermDeviceRead),
	RoleOperator: set(PermAlertPublish, PermAlertRead, PermDeviceRead, PermDeviceProvision),
	"viewer":     set(PermAlertRead, PermDeviceRead),
	RoleUser:     set(PermAlertRead, PermDeviceRead),
}

// RoleHasPerm reports whether a role grants a permission.
func RoleHasPerm(role, perm string) bool {
	p, ok := rolePerms[role]
	if !ok {
		return false
	}
	return p[perm]
}

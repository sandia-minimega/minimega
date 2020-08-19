package rbac

type RoleType int

const (
	_ RoleType = iota
	GLOBAL_ADMIN
	GLOBAL_VIEWER
	EXP_ADMIN
	EXP_USER
	EXP_VIEWER
	VM_VIEWER
)

var ROLE_TYPE_TO_NAME = map[RoleType]string{
	GLOBAL_ADMIN:  "Global Admin",
	GLOBAL_VIEWER: "Global Viewer",
	EXP_ADMIN:     "Experiment Admin",
	EXP_USER:      "Experiment User",
	EXP_VIEWER:    "Experiment Viewer",
	VM_VIEWER:     "VM Viewer",
}

var NAME_TO_ROLE_TYPE = map[string]RoleType{
	"Global Admin":      GLOBAL_ADMIN,
	"Global Viewer":     GLOBAL_VIEWER,
	"Experiment Admin":  EXP_ADMIN,
	"Experiment User":   EXP_USER,
	"Experiment Viewer": EXP_VIEWER,
	"VM Viewer":         VM_VIEWER,
}

/*
version: v0
kind: Role
metadata:
	name: Global Admin
spec:
	policies:
	- resources:
		- "*"
		resourceNames:
		- "*"
		verbs:
		- "*"

version: v0
kind: Role
metadata:
	name: Global Viewer
spec:
	policies:
	- resources:
		- "*"
		resourceNames:
		- "*"
		verbs:
		- list
		- get

version: v0
kind: Role
metadata:
	name: Experiment Admin
spec:
	policies:
	- resources:
		- experiments
		- experiments/*
		- vms
		- vms/*
		resourceNames:
		- foo
		- bar
		verbs:
		- list
		- get
		- update
	- resources:
		- disks
		resourceNames:
		- "*"
		verbs:
		- list
	- resources:
		- hosts
		resourceNames:
		- "*"
		verbs:
		- list

version: v0
kind: Role
metadata:
	name: Experiment User
spec:
	policies:
	- resources:
		- experiments
		- experiments/*
		resourceNames:
		- foo
		- bar
		verbs:
		- list
		- get
	- resources:
		- vms
		- vms/*
		resourceNames:
		- foo
		- bar
		verbs:
		- list
		- get
		- patch
	- resources:
		- vms/redeploy
		resourceNames:
		- foo
		- bar
		verbs:
		- update
	- resources:
		- hosts
		resourceNames:
		- "*"
		verbs:
		- list

version: v0
kind: Role
metadata:
	name: Experiment Viewer
spec:
	policies:
	- resources:
		- experiments
		- experiments/*
		- vms
		- vms/*
		resourceNames:
		- foo
		- bar
		verbs:
		- list
		- get
	- resources:
		- hosts
		resourceNames:
		- "*"
		verbs:
		- list

version: v0
kind: Role
metadata:
	name: VM Viewer
spec:
	policies:
	- resources:
		- vms
		resourceNames:
		- foo_*
		- bar_inverter
		verbs:
		- list
	- resources:
		- vms/screenshot
		- vms/vnc
		resourceNames:
		- foo_*
		- bar_inverter
		verbs:
		- get
*/

func CreateBasePoliciesForRole(role string) Policies {
	roleType := NAME_TO_ROLE_TYPE[role]

	switch roleType {
	case GLOBAL_ADMIN:
		return Policies([]*Policy{
			{
				Resources:     []string{"*", "*/*"},
				ResourceNames: []string{"*"},
				Verbs:         []string{"*"},
			},
		})
	case GLOBAL_VIEWER:
		return Policies([]*Policy{
			{
				Resources:     []string{"*", "*/*"},
				ResourceNames: []string{"*"},
				Verbs:         []string{"list", "get"},
			},
		})
	case EXP_ADMIN:
		// must supply experiment names as resource names or nothing will be allowed
		return Policies([]*Policy{
			{
				Resources: []string{"experiments", "experiments/*"},
				Verbs:     []string{"list", "get", "update"},
			},
			{
				Resources: []string{"vms", "vms/*"},
				Verbs:     []string{"list", "get", "create", "update", "patch", "delete"},
			},
			{
				Resources:     []string{"disks"},
				ResourceNames: []string{"*"},
				Verbs:         []string{"list"},
			},
			{
				Resources:     []string{"hosts"},
				ResourceNames: []string{"*"},
				Verbs:         []string{"list"},
			},
		})
	case EXP_USER: // EXP_VIEWER + VM restart + VM update + VM capture
		// must supply experiment names as resource names or nothing will be allowed
		return Policies([]*Policy{
			{
				Resources: []string{"experiments", "experiments/*"},
				Verbs:     []string{"list", "get"},
			},
			{
				Resources: []string{"vms", "vms/*"},
				Verbs:     []string{"list", "get", "patch"},
			},
			{
				Resources: []string{"vms/redeploy"},
				Verbs:     []string{"update"},
			},
			{
				Resources: []string{"vms/captures"},
				Verbs:     []string{"create", "delete"},
			},
			{
				Resources: []string{"vms/snapshots"},
				Verbs:     []string{"list", "create", "update"},
			},
			{
				Resources:     []string{"hosts"},
				ResourceNames: []string{"*"},
				Verbs:         []string{"list"},
			},
		})
	case EXP_VIEWER:
		// must supply experiment names as resource names or nothing will be allowed
		return Policies([]*Policy{
			{
				Resources: []string{"experiments", "experiments/*", "vms", "vms/*"},
				Verbs:     []string{"list", "get"},
			},
			{
				Resources:     []string{"hosts"},
				ResourceNames: []string{"*"},
				Verbs:         []string{"list"},
			},
		})
	case VM_VIEWER:
		// must supply vm names as resource names or nothing will be allowed
		return Policies([]*Policy{
			{
				Resources: []string{"vms"},
				Verbs:     []string{"list"},
			},
			{
				Resources: []string{"vms/screenshot", "vms/vnc"},
				Verbs:     []string{"get"},
			},
		})
	}

	return nil
}

func NameForRoleType(role RoleType) string {
	return ROLE_TYPE_TO_NAME[role]
}

func RoleTypeForName(name string) RoleType {
	return NAME_TO_ROLE_TYPE[name]
}

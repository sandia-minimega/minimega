package types

type Status string

const (
	StatusStopping     = "stopping"
	StatusStopped      = "stopped"
	StatusStarting     = "starting"
	StatusStarted      = "started"
	StatusCreating     = "creating"
	StatusDeleting     = "deleting"
	StatusRedeploying  = "redeploying"
	StatusSnapshotting = "snapshotting"
	StatusRestoring    = "restoring"
	StatusCommitting   = "committing"
)

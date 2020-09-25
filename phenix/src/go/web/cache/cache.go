package cache

import (
	"time"
)

type Status string

const (
	StatusStopping     Status = "stopping"
	StatusStopped      Status = "stopped"
	StatusStarting     Status = "starting"
	StatusStarted      Status = "started"
	StatusCreating     Status = "creating"
	StatusDeleting     Status = "deleting"
	StatusRedeploying  Status = "redeploying"
	StatusSnapshotting Status = "snapshotting"
	StatusRestoring    Status = "restoring"
	StatusCommitting   Status = "committing"
)

var DefaultCache Cache = NewGoCache()

type Cache interface {
	Get(string) ([]byte, bool)
	Set(string, []byte) error

	SetWithExpire(string, []byte, time.Duration) error

	Lock(string, Status, time.Duration) Status
	Locked(string) Status
	Unlock(string)
}

func Get(key string) ([]byte, bool) {
	return DefaultCache.Get(key)
}

func Set(key string, val []byte) error {
	return DefaultCache.Set(key, val)
}

func SetWithExpire(key string, val []byte, exp time.Duration) error {
	return DefaultCache.SetWithExpire(key, val, exp)
}

func Lock(key string, status Status, exp time.Duration) Status {
	return DefaultCache.Lock(key, status, exp)
}

func Locked(key string) Status {
	return DefaultCache.Locked(key)
}

func Unlock(key string) {
	DefaultCache.Unlock(key)
}

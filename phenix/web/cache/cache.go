package cache

import (
	"time"

	"phenix/web/types"
)

var DefaultCache Cache = NewGoCache()

type Cache interface {
	Get(string) ([]byte, bool)
	Set(string, []byte) error

	SetWithExpire(string, []byte, time.Duration) error

	Lock(string, types.Status, time.Duration) types.Status
	Locked(string) types.Status
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

func Lock(key string, status types.Status, exp time.Duration) types.Status {
	return DefaultCache.Lock(key, status, exp)
}

func Locked(key string) types.Status {
	return DefaultCache.Locked(key)
}

func Unlock(key string) {
	DefaultCache.Unlock(key)
}

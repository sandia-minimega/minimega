package cache

import (
	"time"

	gocache "github.com/patrickmn/go-cache"
)

type GoCache struct {
	c *gocache.Cache
}

func NewGoCache() *GoCache {
	return &GoCache{c: gocache.New(gocache.NoExpiration, 30*time.Second)}
}

func (this GoCache) Get(key string) ([]byte, bool) {
	v, ok := this.c.Get(key)
	if !ok {
		return nil, false
	}

	return v.([]byte), true
}

func (this *GoCache) Set(key string, val []byte) error {
	this.c.Set(key, val, -1)
	return nil
}

func (this *GoCache) SetWithExpire(key string, val []byte, exp time.Duration) error {
	this.c.Set(key, val, exp)
	return nil
}

func (this *GoCache) Lock(key string, status Status, exp time.Duration) Status {
	key = "LOCK|" + key

	if err := this.c.Add(key, status, exp); err != nil {
		v, ok := this.c.Get(key)

		// This *might* happen if the key expires or is deleted between
		// calling `Add` and `Get`.
		if !ok {
			return ""
		}

		return v.(Status)
	}

	return ""
}

func (this *GoCache) Locked(key string) Status {
	key = "LOCK|" + key

	v, ok := this.c.Get(key)
	if !ok {
		return ""
	}

	return v.(Status)
}

func (this *GoCache) Unlock(key string) {
	this.c.Delete("LOCK|" + key)
}

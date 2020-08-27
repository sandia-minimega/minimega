package store

import (
	"fmt"
	"net/url"
	"phenix/types"
)

var DefaultStore Store = NewBoltDB()

func Init(opts ...Option) error {
	options := NewOptions(opts...)

	u, err := url.Parse(options.Endpoint)
	if err != nil {
		return fmt.Errorf("parsing store endpoint: %w", err)
	}

	switch u.Scheme {
	case "bolt":
		DefaultStore = NewBoltDB()
	case "etcd":
		DefaultStore = NewEtcd()
	default:
		return fmt.Errorf("unknown store scheme '%s'", u.Scheme)
	}

	return DefaultStore.Init(opts...)
}

func Close() error {
	return DefaultStore.Close()
}

func List(kinds ...string) (types.Configs, error) {
	return DefaultStore.List(kinds...)
}

func Get(config *types.Config) error {
	return DefaultStore.Get(config)
}

func Create(config *types.Config) error {
	return DefaultStore.Create(config)
}

func Update(config *types.Config) error {
	return DefaultStore.Update(config)
}

func Patch(config *types.Config, data map[string]interface{}) error {
	return DefaultStore.Patch(config, data)
}

func Delete(config *types.Config) error {
	return DefaultStore.Delete(config)
}

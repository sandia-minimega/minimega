package store

import (
	"phenix/types"
)

var DefaultStore Store = NewBoltDB()

func Init(opts ...Option) error {
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

func Patch(kind, name string, data map[string]interface{}) error {
	return DefaultStore.Patch(kind, name, data)
}

func Delete(kind, name string) error {
	return DefaultStore.Delete(kind, name)
}

package store

import (
	"phenix/types"
)

type Store interface {
	Init(...Option) error
	Close() error

	List(...string) (types.Configs, error)
	Get(*types.Config) error
	Create(*types.Config) error
	Update(*types.Config) error
	Patch(string, string, map[string]interface{}) error
	Delete(string, string) error
}

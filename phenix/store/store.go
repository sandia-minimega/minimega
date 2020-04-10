package store

import "phenix/types"

type Store interface {
	Init(...Option) error
	Close() error

	Get(*types.Config) error
	Create(*types.Config) error
	Update(*types.Config) error
	Patch(string, string, map[string]interface{}) error
	Delete(string, string) error
}

type TopologyStore interface {
	GetTopology(string, interface{}) error
	CreateTopology(string, interface{}) error
	UpdateTopology(string, interface{}) error
	PatchTopology(string, interface{}) error
	DeleteTopology(string)
}

package store

type Store interface {
	TopologyStore
}

type TopologyStore interface {
	GetTopology(string, interface{}) error
	CreateTopology(string, interface{}) error
	UpdateTopology(string, interface{}) error
	PatchTopology(string, interface{}) error
	DeleteTopology(string)
}

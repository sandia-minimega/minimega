package store

// Store is the interface that identifies all the required functionality for a
// config store. Not all functions are required to be implemented. If not
// implemented, they should return an error stating such.
type Store interface {
	// Init is used to initialize a config store with options generic to all store
	// implementations.
	Init(...Option) error

	// Close persists any queued writes and closes the store.
	Close() error

	// List returns a list of configs of the given kind(s) from the store.
	List(...string) (Configs, error)

	// Get initializes the given config with data from the store.
	Get(*Config) error

	// Create persists the given config to the store if it doesn't already exist.
	Create(*Config) error

	// Update persists the given config to the store if it already exists.
	Update(*Config) error

	// Patch modifies the given config in the store with the given data if the
	// config already exists.
	Patch(*Config, map[string]interface{}) error

	// Delete removes the given config from the config store.
	Delete(*Config) error
}

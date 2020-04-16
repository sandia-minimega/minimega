package store

import (
	"encoding/json"
	"fmt"
	"time"

	"phenix/types"

	"go.etcd.io/bbolt"
)

type BoltDB struct {
	db *bbolt.DB
}

func NewBoltDB() Store {
	return new(BoltDB)
}

func (this *BoltDB) Init(opts ...Option) error {
	options := NewOptions(opts...)

	var err error

	this.db, err = bbolt.Open(options.Endpoint, 0600, &bbolt.Options{NoFreelistSync: true})
	if err != nil {
		return fmt.Errorf("opening Bolt file: %w", err)
	}

	return nil
}

func (this BoltDB) Close() error {
	return this.db.Close()
}

func (this BoltDB) List(kinds ...string) (types.Configs, error) {
	var configs types.Configs

	for _, kind := range kinds {
		if err := this.ensureBucket(kind); err != nil {
			return nil, err
		}

		err := this.db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte(kind))

			err := b.ForEach(func(_, v []byte) error {
				var c types.Config

				if err := json.Unmarshal(v, &c); err != nil {
					return fmt.Errorf("unmarshaling config JSON: %w", err)
				}

				configs = append(configs, c)

				return nil
			})

			if err != nil {
				return fmt.Errorf("iterating %s bucket: %w", kind, err)
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("getting configs from store: %w", err)
		}
	}

	return configs, nil
}

func (this BoltDB) Get(c *types.Config) error {
	v, err := this.get(c.Kind, c.Metadata.Name)
	if err != nil {
		return fmt.Errorf("getting config: %w", err)
	}

	if err := json.Unmarshal(v, c); err != nil {
		return fmt.Errorf("unmarshaling config JSON: %w", err)
	}

	return nil
}

func (this BoltDB) Create(c *types.Config) error {
	if _, err := this.get(c.Kind, c.Metadata.Name); err == nil {
		return fmt.Errorf("config %s/%s already exists", c.Kind, c.Metadata.Name)
	}

	now := time.Now()

	c.Metadata.Created = now
	c.Metadata.Updated = now

	v, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config JSON: %w", err)
	}

	if err := this.put(c.Kind, c.Metadata.Name, v); err != nil {
		return fmt.Errorf("writing config JSON to Bolt: %w", err)
	}

	return nil
}

func (this BoltDB) Update(c *types.Config) error {
	if _, err := this.get(c.Kind, c.Metadata.Name); err != nil {
		return fmt.Errorf("config does not exist")
	}

	c.Metadata.Updated = time.Now()

	v, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config JSON: %w", err)
	}

	if err := this.put(c.Kind, c.Metadata.Name, v); err != nil {
		return fmt.Errorf("writing config JSON to Bolt: %w", err)
	}

	return nil
}

func (this BoltDB) Patch(string, string, map[string]interface{}) error {
	return fmt.Errorf("BoltDB.Patch not implemented")
}

func (this BoltDB) Delete(kind, name string) error {
	if err := this.ensureBucket(kind); err != nil {
		return nil
	}

	err := this.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(kind))
		return b.Delete([]byte(name))
	})

	if err != nil {
		return fmt.Errorf("deleting key %s in bucket %s: %w", name, kind, err)
	}

	return nil
}

func (this BoltDB) get(b, k string) ([]byte, error) {
	if err := this.ensureBucket(b); err != nil {
		return nil, err
	}

	var v []byte

	this.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(b))
		v = b.Get([]byte(k))
		return nil
	})

	if v == nil {
		return nil, fmt.Errorf("key %s does not exist in bucket %s", k, b)
	}

	return v, nil
}

func (this BoltDB) put(b, k string, v []byte) error {
	if err := this.ensureBucket(b); err != nil {
		return err
	}

	err := this.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(b))
		return b.Put([]byte(k), v)
	})

	if err != nil {
		return fmt.Errorf("updating value for key %s in bucket %s: %w", k, b, err)
	}

	return nil
}

func (this BoltDB) ensureBucket(name string) error {
	return this.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return fmt.Errorf("creating bucket in Bolt: %w", err)
		}

		return nil
	})
}

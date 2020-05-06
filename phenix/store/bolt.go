package store

import (
	"encoding/json"
	"fmt"
	"net/url"
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

	u, err := url.Parse(options.Endpoint)
	if err != nil {
		return fmt.Errorf("parsing BoltDB endpoint: %w", err)
	}

	if u.Scheme != "bolt" {
		return fmt.Errorf("invalid scheme '%s' for BoltDB endpoint", u.Scheme)
	}

	this.db, err = bbolt.Open(u.Host+u.Path, 0600, &bbolt.Options{NoFreelistSync: true})
	if err != nil {
		return fmt.Errorf("opening BoltDB file: %w", err)
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

	now := time.Now().Format(time.RFC3339)

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

	c.Metadata.Updated = time.Now().Format(time.RFC3339)

	v, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config JSON: %w", err)
	}

	if err := this.put(c.Kind, c.Metadata.Name, v); err != nil {
		return fmt.Errorf("writing config JSON to Bolt: %w", err)
	}

	return nil
}

func (this BoltDB) Patch(*types.Config, map[string]interface{}) error {
	return fmt.Errorf("BoltDB.Patch not implemented")
}

func (this BoltDB) Delete(c *types.Config) error {
	if err := this.ensureBucket(c.Kind); err != nil {
		return nil
	}

	err := this.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(c.Kind))
		return b.Delete([]byte(c.Metadata.Name))
	})

	if err != nil {
		return fmt.Errorf("deleting key %s in bucket %s: %w", c.Metadata.Name, c.Kind, err)
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

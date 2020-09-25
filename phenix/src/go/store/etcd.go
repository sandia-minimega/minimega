package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"phenix/types"
	"strings"
	"time"

	"go.etcd.io/etcd/v3/clientv3"
)

type Etcd struct {
	endpoints []string

	cli *clientv3.Client
}

func NewEtcd() Store {
	return new(Etcd)
}

func (this *Etcd) Init(opts ...Option) error {
	options := NewOptions(opts...)

	u, err := url.Parse(options.Endpoint)
	if err != nil {
		return fmt.Errorf("parsing Etcd endpoint: %w", err)
	}

	if u.Scheme != "etcd" {
		return fmt.Errorf("invalid scheme '%s' for Etcd endpoint", u.Scheme)
	}

	this.endpoints = []string{u.Host + u.Path}

	cfg := clientv3.Config{
		Endpoints: []string{u.Host + u.Path},
	}

	this.cli, err = clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("creating new Etcd client: %w", err)
	}

	return nil
}

func (this Etcd) Close() error {
	return this.cli.Close()
}

func (this Etcd) List(kinds ...string) (types.Configs, error) {
	var configs types.Configs

	for _, kind := range kinds {
		kind = strings.ToLower(kind)

		resp, err := this.cli.Get(context.Background(), kind, clientv3.WithPrefix())
		if err != nil {
			return nil, fmt.Errorf("getting list of configs from Etcd: %w", err)
		}

		for _, e := range resp.Kvs {
			var c types.Config

			if err := json.Unmarshal(e.Value, &c); err != nil {
				return nil, fmt.Errorf("unmarshaling config JSON: %w", err)
			}

			configs = append(configs, c)
		}
	}

	return configs, nil
}

func (this Etcd) Get(c *types.Config) error {
	key := fmt.Sprintf("%s/%s", strings.ToLower(c.Kind), c.Metadata.Name)

	resp, err := this.cli.Get(context.Background(), key)
	if err != nil {
		return fmt.Errorf("getting config %s from Etcd: %w", key, err)
	}

	if resp.Count == 0 {
		return fmt.Errorf("config %s not found", key)
	}

	e := resp.Kvs[0]

	if err := json.Unmarshal(e.Value, &c); err != nil {
		return fmt.Errorf("unmarshaling config JSON: %w", err)
	}

	return nil
}

func (this Etcd) Create(c *types.Config) error {
	key := fmt.Sprintf("%s/%s", strings.ToLower(c.Kind), c.Metadata.Name)

	if resp, _ := this.cli.Get(context.Background(), key); resp.Count != 0 {
		return fmt.Errorf("config %s/%s already exists", c.Kind, c.Metadata.Name)
	}

	now := time.Now().Format(time.RFC3339)

	c.Metadata.Created = now
	c.Metadata.Updated = now

	v, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config JSON: %w", err)
	}

	if _, err := this.cli.Put(context.Background(), key, string(v)); err != nil {
		return fmt.Errorf("writing config JSON to Etcd: %w", err)
	}

	return nil
}

func (this Etcd) Update(c *types.Config) error {
	key := fmt.Sprintf("%s/%s", strings.ToLower(c.Kind), c.Metadata.Name)

	if resp, _ := this.cli.Get(context.Background(), key); resp.Count == 0 {
		return fmt.Errorf("config %s/%s doesn't exist", c.Kind, c.Metadata.Name)
	}

	now := time.Now().Format(time.RFC3339)

	c.Metadata.Updated = now

	v, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config JSON: %w", err)
	}

	if _, err := this.cli.Put(context.Background(), key, string(v)); err != nil {
		return fmt.Errorf("writing config JSON to Etcd: %w", err)
	}

	return nil
}

func (this Etcd) Patch(c *types.Config, u map[string]interface{}) error {
	return fmt.Errorf("not implemented")
}

func (this Etcd) Delete(c *types.Config) error {
	key := fmt.Sprintf("%s/%s", strings.ToLower(c.Kind), c.Metadata.Name)

	if _, err := this.cli.Delete(context.Background(), key); err != nil {
		return fmt.Errorf("deleting key %s: %w", key, err)
	}

	return nil
}

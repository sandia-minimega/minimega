package rbac

import (
	"fmt"

	"phenix/api/config"
	"phenix/store"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

/*
version: v0
kind: User
metadata:
	name: <username>
spec:
	username: <username>
	password: <bas64 encoded password>
	firstName: <first name>
	lastName: <last name>
	rbac:
		roleName: <role name>
		policies:
		- resources:
			- vms
			resourceNames:
			- foo_*
			- bar_inverter
			verbs:
			- list
		- resources:
			- vms/screenshot
			- vms/vnc
			resourceNames:
			- foo_*
			- bar_inverter
			verbs:
			- get
*/

type User struct {
	Spec *v1.UserSpec

	config *types.Config
}

func NewUser(u, p string) *User {
	spec := &v1.UserSpec{
		Username: u,
		Password: p,
	}

	c := &types.Config{
		Version:  "phenix.sandia.gov/v1",
		Kind:     "User",
		Metadata: types.ConfigMetadata{Name: u},
		Spec:     structs.MapDefaultCase(spec, structs.CASESNAKE),
	}

	if err := store.Create(c); err != nil {
		return nil
	}

	return &User{Spec: spec, config: c}
}

func GetUsers() ([]*User, error) {
	configs, err := config.List("user")
	if err != nil {
		return nil, fmt.Errorf("getting user configs: %w", err)
	}

	users := make([]*User, len(configs))

	for i, c := range configs {
		var u v1.UserSpec
		if err := mapstructure.Decode(c.Spec, &u); err != nil {
			return nil, fmt.Errorf("decoding user config: %w", err)
		}

		users[i] = &User{Spec: &u}
	}

	return users, nil
}

func GetUser(uname string) (*User, error) {
	c, err := config.Get("user/" + uname)
	if err != nil {
		return nil, fmt.Errorf("getting user config: %w", err)
	}

	var u v1.UserSpec
	if err := mapstructure.Decode(c.Spec, &u); err != nil {
		return nil, fmt.Errorf("decoding user config: %w", err)
	}

	return &User{Spec: &u, config: c}, nil
}

func (this User) Username() string {
	return this.Spec.Username
}

func (this User) FirstName() string {
	return this.Spec.FirstName
}

func (this User) LastName() string {
	return this.Spec.LastName
}

func (this User) RoleName() string {
	if this.Spec.Role == nil {
		return ""
	}

	return this.Spec.Role.Name
}

func (this User) AddToken(token, note string) error {
	if this.Spec.Tokens == nil {
		this.Spec.Tokens = make(map[string]string)
	}

	this.Spec.Tokens[token] = note
	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("persisting new user token: %w", err)
	}

	return nil
}

func (this User) DeleteToken(token string) error {
	delete(this.Spec.Tokens, token)

	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("deleting user token: %w", err)
	}

	return nil
}

func (this User) ValidateToken(token string) error {
	for t := range this.Spec.Tokens {
		if token == t {
			return nil
		}
	}

	return fmt.Errorf("token not found for user")
}

func (this User) Save() error {
	if err := store.Update(this.config); err != nil {
		return fmt.Errorf("updating user in store: %w", err)
	}

	return nil
}

func (this User) Role() (Role, error) {
	if this.Spec.Role == nil {
		return Role{}, fmt.Errorf("user has no role assigned")
	}

	return Role{Spec: this.Spec.Role}, nil
}

func (this *User) SetRole(role *Role) error {
	this.Spec.Role = role.Spec
	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("setting user role: %w", err)
	}

	return nil
}

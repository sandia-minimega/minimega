package rbac

import (
	"fmt"

	"phenix/api/config"
	"phenix/store"
	"phenix/types"

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

type UserSpec struct {
	Username  string `yaml:"username" json:"username"`
	Password  string `yaml:"password" json:"password"`
	FirstName string `yaml:"firstName" json:"first_name"`
	LastName  string `yaml:"lastName" json:"last_name"`
	Role      *Role  `yaml:"rbac" json:"rbac" structs:"rbac" mapstructure:"rbac"`

	Tokens map[string]string `yaml:"tokens" json:"tokens"`
}

type User struct {
	Spec *UserSpec

	config *types.Config
}

func GetUser(uname string) (*User, error) {
	c, err := config.Get("user/" + uname)
	if err != nil {
		return nil, fmt.Errorf("getting user config: %w", err)
	}

	var u UserSpec
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

	if err := store.Update(this.config); err != nil {
		return fmt.Errorf("persisting new user token: %w", err)
	}

	return nil
}

func (this User) DeleteToken(token string) error {
	delete(this.Spec.Tokens, token)

	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := store.Update(this.config); err != nil {
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

func (this User) GetRole() (*Role, error) {
	if this.Spec.Role == nil {
		return nil, fmt.Errorf("user has no role assigned")
	}

	this.Spec.Role.MapPolicies()

	this.Spec.Role.AddPolicies(&Policy{
		Resources:     []string{"users"},
		ResourceNames: []string{this.Spec.Username},
		Verbs:         []string{"get"},
	})

	return this.Spec.Role, nil
}

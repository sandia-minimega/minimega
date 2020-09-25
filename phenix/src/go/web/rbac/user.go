package rbac

import (
	"encoding/base64"
	"fmt"

	"phenix/api/config"
	"phenix/store"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/crypto/bcrypt"
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

var ErrPasswordInvalid = fmt.Errorf("password invalid")

type User struct {
	Spec *v1.UserSpec

	config *types.Config
}

func NewUser(u, p string) *User {
	hashed, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return nil
	}

	spec := &v1.UserSpec{
		Username: u,
		Password: string(hashed),
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

func (this User) UpdateFirstName(name string) error {
	this.Spec.FirstName = name

	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("updating user first name: %w", err)
	}

	return nil
}

func (this User) UpdateLastName(name string) error {
	this.Spec.LastName = name

	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("updating user last name: %w", err)
	}

	return nil
}

func (this User) AddToken(token, note string) error {
	if this.Spec.Tokens == nil {
		this.Spec.Tokens = make(map[string]string)
	}

	enc := base64.StdEncoding.EncodeToString([]byte(token))

	this.Spec.Tokens[enc] = note
	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("persisting new user token: %w", err)
	}

	return nil
}

func (this User) DeleteToken(token string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(token))

	delete(this.Spec.Tokens, enc)

	this.config.Spec = structs.MapDefaultCase(this.Spec, structs.CASESNAKE)

	if err := this.Save(); err != nil {
		return fmt.Errorf("deleting user token: %w", err)
	}

	return nil
}

func (this User) ValidateToken(token string) error {
	for enc := range this.Spec.Tokens {
		t, _ := base64.StdEncoding.DecodeString(enc)

		if token == string(t) {
			return nil
		}
	}

	return fmt.Errorf("token not found for user")
}

func (this User) ValidatePassword(p string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(this.Spec.Password), []byte(p)); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return ErrPasswordInvalid
		}

		return err
	}

	return nil
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

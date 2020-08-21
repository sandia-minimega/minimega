package v1

type UserSpec struct {
	Username  string    `yaml:"username" json:"username" structs:"username" mapstructure:"username"`
	Password  string    `yaml:"password" json:"password" structs:"password" mapstructure:"password"`
	FirstName string    `yaml:"firstName" json:"first_name" structs:"first_name" mapstructure:"first_name"`
	LastName  string    `yaml:"lastName" json:"last_name" structs:"last_name" mapstructure:"last_name"`
	Role      *RoleSpec `yaml:"rbac" json:"rbac" structs:"rbac" mapstructure:"rbac"`

	Tokens map[string]string `yaml:"tokens" json:"tokens" structs:"tokens" mapstructure:"tokens"`
}

package types

type SignupUser struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type User struct {
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	FirstName    string   `json:"first_name"`
	LastName     string   `json:"last_name"`
	RoleName     string   `json:"role_name"`
	ResourceName []string `json:"resource_names"`
}

type CreateUser struct {
	User User `json:"user"`
}

type UpdateUser struct {
	User User `json:"user"`
}

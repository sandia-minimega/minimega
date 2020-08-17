package database

import (
	"fmt"

	gdb "gophenix/database"
	"gophenix/web/rbac"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

var errPasswordInvalid = errors.New("password invalid")

func GetUsers() ([]rbac.User, error) {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return nil, errors.Wrap(err, "getting phenix database connection")
	}

	rows, err := pdb.Conn().Query(
		"SELECT id, username, first_name, last_name, role FROM users",
	)

	if err != nil {
		return nil, errors.Wrap(err, "querying users from database")
	}

	defer rows.Close()

	var users []rbac.User

	for rows.Next() {
		var u rbac.User
		rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Role)
		users = append(users, u)
	}

	if rows.Err() != nil {
		return nil, errors.Wrap(err, "processing users from database")
	}

	return users, nil
}

func GetUser(username string) (rbac.User, error) {
	var user rbac.User

	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return user, errors.Wrap(err, "getting phenix database connection")
	}

	err = pdb.Conn().QueryRow(
		"SELECT id, username, first_name, last_name, role FROM users WHERE username = $1",
		username,
	).Scan(&user.ID, &user.Username, &user.FirstName, &user.LastName, &user.Role)

	if err != nil {
		return user, errors.Wrapf(err, "querying user %s from database", username)
	}

	return user, nil
}

func AddUser(u *rbac.User) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(err, "hashing new user password")
	}

	u.Password = ""

	err = pdb.Conn().QueryRow(
		`INSERT INTO users (username, password, first_name, last_name, role)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		u.Username, hashed, u.FirstName, u.LastName, u.Role,
	).Scan(&u.ID)

	if err != nil {
		return errors.Wrap(err, "inserting user into database")
	}

	return nil
}

func UpsertUser(u *rbac.User) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(err, "hashing new user password")
	}

	u.Password = ""

	err = pdb.Conn().QueryRow(
		`INSERT INTO users (username, password, first_name, last_name, role)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (username) DO UPDATE SET
				password   = EXCLUDED.password,
				first_name = EXCLUDED.first_name,
				last_name  = EXCLUDED.last_name,
				role       = EXCLUDED.role
			RETURNING id`,
		u.Username, hashed, u.FirstName, u.LastName, u.Role,
	).Scan(&u.ID)

	if err != nil {
		return errors.Wrap(err, "inserting user into database")
	}

	return nil
}

func UpdateUserSetting(username string, setting string, value interface{}) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	query := fmt.Sprintf("UPDATE users SET %s = $1 WHERE username = $2", setting)

	if _, err := pdb.Conn().Exec(query, value, username); err != nil {
		return errors.Wrapf(err, "updating %s for user %s", setting, username)
	}

	return nil
}

func DeleteUser(username string) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	_, err = pdb.Conn().Exec(
		"DELETE FROM user_tokens WHERE username = $1",
		username,
	)

	if err != nil {
		return errors.Wrapf(err, "deleting tokens for user %s", username)
	}

	_, err = pdb.Conn().Exec(
		"DELETE FROM users WHERE username = $1",
		username,
	)

	if err != nil {
		return errors.Wrapf(err, "deleting user %s", username)
	}

	return nil
}

func ValidateUserPassword(username, password string) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	var hashed string

	err = pdb.Conn().QueryRow(
		"SELECT password FROM users WHERE username = $1",
		username,
	).Scan(&hashed)

	if err != nil {
		return errors.Wrapf(err, "querying hashed password for user %s from database", username)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return errPasswordInvalid
		}

		return err
	}

	return nil
}

func AddUserToken(username, token string) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	_, err = pdb.Conn().Exec(
		"INSERT INTO user_tokens (username, token) VALUES ($1, $2)",
		username, token,
	)

	if err != nil {
		return errors.Wrapf(err, "adding token for user %s to database", username)
	}

	return nil
}

func DeleteUserToken(token string) error {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return errors.Wrap(err, "getting phenix database connection")
	}

	_, err = pdb.Conn().Exec(
		"DELETE FROM user_tokens WHERE token = $1",
		token,
	)

	if err != nil {
		return errors.Wrap(err, "deleting token from database")
	}

	return nil
}

func UserTokenExists(username, token string) (bool, error) {
	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return false, errors.Wrap(err, "getting phenix database connection")
	}

	var count int

	err = pdb.Conn().QueryRow(
		"SELECT count(*) FROM user_tokens WHERE username = $1 AND token = $2",
		username, token,
	).Scan(&count)

	if err != nil {
		return false, errors.Wrapf(err, "querying tokens for user %s from database", username)
	}

	return count != 0, nil
}

func GetUserRole(username string) (rbac.Role, error) {
	var role rbac.Role

	pdb, err := gdb.NewPhenixDB()
	if err != nil {
		return role, errors.Wrap(err, "getting phenix database connection")
	}

	err = pdb.Conn().QueryRow(
		"SELECT role FROM users WHERE username = $1",
		username,
	).Scan(&role)

	if err != nil {
		return role, errors.Wrap(err, "querying user role from database")
	}

	return role, nil
}

package command

import (
	"botDashboard/internal/store"
)

type UserSetAdmin struct {
	Email string
}

func (r *UserSetAdmin) Execute() string {
	u := store.GetUserRepository()
	data, err := u.FindUserByEmail(r.Email)
	if err != nil {
		return err.Error()
	}
	data.IsAdmin = true
	err = u.UpdateUser(data, "")
	if err != nil {
		return err.Error()
	}
	return "Success"
}

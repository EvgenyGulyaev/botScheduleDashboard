package command

import (
	"botDashboard/internal/store"
	"strings"
)

type UserSetSuperAdmin struct {
	Identity string
}

func (r *UserSetSuperAdmin) Execute() string {
	u := store.GetUserRepository()
	identity := strings.TrimSpace(r.Identity)
	data, err := u.FindUserByEmail(identity)
	if err != nil {
		data, err = u.FindUserByLogin(identity)
	}
	if err != nil {
		return err.Error()
	}
	data.IsAdmin = true
	data.IsSuperAdmin = true
	data.AppPermissions = nil
	err = u.UpdateUser(data, "")
	if err != nil {
		return err.Error()
	}
	return "Success"
}

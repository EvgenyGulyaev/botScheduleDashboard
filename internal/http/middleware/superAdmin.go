package middleware

import (
	"sync"

	"github.com/go-www/silverlining"
)

type SuperAdministrator struct {
}

var instanceSuperAdmin *SuperAdministrator

var onceSuperAdmin sync.Once

func GetSuperAdministrator() *SuperAdministrator {
	onceSuperAdmin.Do(func() {
		instanceSuperAdmin = &SuperAdministrator{}
	})
	return instanceSuperAdmin
}

func (s *SuperAdministrator) Check(next func(c *silverlining.Context)) func(c *silverlining.Context) {
	return func(c *silverlining.Context) {
		auth := GetAuth()
		data, err := auth.getUserByToken(c)
		if err != nil {
			handleError(c, err.Error())
			return
		}
		if !data.IsSuperAdmin {
			handleError(c, "User is not super admin")
			return
		}

		next(c)
	}
}

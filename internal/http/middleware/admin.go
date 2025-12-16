package middleware

import (
	"sync"

	"github.com/go-www/silverlining"
)

type Administrator struct {
}

var instanceAdmin *Administrator

var onceAdmin sync.Once

func GetAdministrator() *Administrator {
	onceAdmin.Do(func() {
		instanceAdmin = &Administrator{}
	})
	return instanceAdmin
}

func (s *Administrator) Check(next func(c *silverlining.Context)) func(c *silverlining.Context) {
	return func(c *silverlining.Context) {
		auth := GetAuth()
		data, err := auth.getUserByToken(c)
		if err != nil {
			handleError(c, err.Error())
			return
		}
		if data.IsAdmin == false {
			handleError(c, "User is not admin")
		}

		next(c)
	}
}

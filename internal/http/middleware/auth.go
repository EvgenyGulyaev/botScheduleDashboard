package middleware

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"sync"

	"github.com/go-www/silverlining"
)

type Authorize struct {
}

var instanceAuth *Authorize

var onceAuth sync.Once

func GetAuth() *Authorize {
	onceAuth.Do(func() {
		instanceAuth = &Authorize{}
	})
	return instanceAuth
}

func (s *Authorize) Check(next func(c *silverlining.Context)) func(c *silverlining.Context) {
	return func(c *silverlining.Context) {
		_, err := s.getUserByToken(c)
		if err != nil {
			handleError(c, err.Error())
			return
		}
		next(c)
	}
}

func (s *Authorize) getUserByToken(ctx *silverlining.Context) (model.UserData, error) {
	j := GetJwt()
	email, err := j.getEmailByToken(ctx)
	if err != nil {
		return model.UserData{}, err
	}

	r := store.GetUserRepository()
	data, err := r.FindUserByEmail(email)
	if err != nil {
		return model.UserData{}, err
	}

	return data, err
}

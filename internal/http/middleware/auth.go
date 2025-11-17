package middleware

import (
	"botDashboard/internal/store"
	"github.com/go-www/silverlining"
	"sync"
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

func (s *Authorize) getUserByToken(ctx *silverlining.Context) (store.UserData, error) {
	j := GetJwt()
	email, err := j.getEmailByToken(ctx)
	if err != nil {
		return store.UserData{}, err
	}

	u := store.GetUser()
	data, err := u.FindUserByEmail(email)
	if err != nil {
		return store.UserData{}, err
	}

	return data, err
}

package middleware

import (
	"github.com/go-www/silverlining"
	"log"
	"net/http"
)

type Hoc interface {
	Check(func(c *silverlining.Context)) func(c *silverlining.Context)
}

const (
	Token string = "jwt"
	Auth  string = "auth"
	Admin string = "admin"
)

var keys = map[string]Hoc{
	Token: GetJwt(),
	Auth:  GetAuth(),
	Admin: GetAdministrator(),
}

func Use(ms []string, finalHandler func(c *silverlining.Context)) func(c *silverlining.Context) {
	h := finalHandler
	for i := len(ms) - 1; i >= 0; i-- {
		mw, ok := keys[ms[i]]
		if ok {
			next := h
			h = mw.Check(func(c *silverlining.Context) {
				next(c)
			})
		}
	}
	return h
}

func handleError(ctx *silverlining.Context, value string) {
	err := ctx.WriteJSON(http.StatusUnauthorized, value)
	if err != nil {
		log.Print(err)
	}
}

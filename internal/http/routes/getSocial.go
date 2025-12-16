package routes

import (
	"botDashboard/internal/store"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetSocialUser(ctx *silverlining.Context) {
	r := store.GetSocialUserRepository()
	data, err := r.ListAll()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
	}

	err = ctx.WriteJSON(http.StatusOK, data)
	if err != nil {
		log.Print(err)
	}
}

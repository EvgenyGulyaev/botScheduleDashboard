package routes

import (
	"botDashboard/internal/http/status"
	"botDashboard/internal/http/validator"
	"botDashboard/internal/model"
	"encoding/json"
	"github.com/go-www/silverlining"
	"log"
)

type bodyPostLogin struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func PostLogin(ctx *silverlining.Context, body []byte) {
	var req bodyPostLogin
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: status.FAIL})
		return
	}

	u := model.GetUser()
	data, err := u.FindUserByEmail(req.Email)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: status.FAIL})
		return
	}

	userValidator := validator.UserPasswordValidator{Hash: data.HashedPassword, Pass: req.Password}
	if userValidator.Validate() == false {
		GetError(ctx, &Error{Message: "Password is invalid", Status: status.FAIL})
		return
	}

	err = ctx.WriteJSON(status.OK, data)
	if err != nil {
		log.Print(err)
	}
}

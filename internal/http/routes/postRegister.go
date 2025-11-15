package routes

import (
	"botDashboard/internal/http/status"
	"botDashboard/internal/http/validator"
	"botDashboard/internal/model"
	"encoding/json"
	"github.com/go-www/silverlining"
	"log"
)

type bodyPostRegister struct {
	Login    string `json:"login"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func PostRegister(ctx *silverlining.Context, body []byte) {
	var req bodyPostRegister
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: status.FAIL})
		return
	}

	mailValidator := validator.UserEmailValidator{Email: req.Email}
	if mailValidator.Validate() == false {
		GetError(ctx, &Error{Message: "Email is invalid", Status: status.FAIL})
		return
	}

	u := model.GetUser()
	data, err := u.CreateUser(req.Login, req.Email, req.Password)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: status.FAIL})
		return
	}

	err = ctx.WriteJSON(status.OK, data)
	if err != nil {
		log.Print(err)
	}
}

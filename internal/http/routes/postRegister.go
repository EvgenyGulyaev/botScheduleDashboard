package routes

import (
	"botDashboard/internal/http/validator"
	"botDashboard/internal/model"
	"botDashboard/pkg/middleware"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type ResponsePostRegister struct {
	token string
	model.UserData
}

type bodyPostRegister struct {
	Login    string `json:"login"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func PostRegister(ctx *silverlining.Context, body []byte) {
	var req bodyPostRegister
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	mailValidator := validator.UserEmailValidator{Email: req.Email}
	if mailValidator.Validate() == false {
		GetError(ctx, &Error{Message: "Email is invalid", Status: http.StatusInternalServerError})
		return
	}

	u := model.GetUser()
	data, err := u.CreateUser(req.Login, req.Email, req.Password)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	token, err := middleware.GetJwt().CreateToken(data.Login)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
	}

	err = ctx.WriteJSON(http.StatusOK, ResponsePostRegister{token: token, UserData: data})
	if err != nil {
		log.Print(err)
	}
}

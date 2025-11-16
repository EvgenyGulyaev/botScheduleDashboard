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

type bodyPostLogin struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ResponsePostLogin struct {
	token string
	model.UserData
}

func PostLogin(ctx *silverlining.Context, body []byte) {
	var req bodyPostLogin
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	u := model.GetUser()
	data, err := u.FindUserByEmail(req.Email)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	userValidator := validator.UserPasswordValidator{Hash: data.HashedPassword, Pass: req.Password}
	if userValidator.Validate() == false {
		GetError(ctx, &Error{Message: "Password is invalid", Status: http.StatusInternalServerError})
		return
	}

	token, err := middleware.GetJwt().CreateToken(data.Login)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
	}

	err = ctx.WriteJSON(http.StatusOK, ResponsePostLogin{token: token, UserData: data})
	if err != nil {
		log.Print(err)
	}
}

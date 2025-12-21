package routes

import (
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"botDashboard/pkg/helpers"
	"encoding/json"
	"log"
	"net/http"
	"runtime"

	"github.com/go-www/silverlining"
)

type PushMessage struct {
	Message string `json:"message"`
	Network string `json:"network"`
}

func PostMessageSendAll(ctx *silverlining.Context, body []byte) {
	var req PushMessage
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	sr := store.GetSocialUserRepository()
	data, err := sr.ListFilterByNet(req.Network)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	go func() {
		helpers.MultipleWork[model.SocialUser, bool](data, runtime.NumCPU(), func(i model.SocialUser) bool {
			m := &producer.Message{User: i.Id, Message: req.Message, Network: req.Network}
			return m.Publish() == nil
		})
	}()

	err = ctx.WriteJSON(http.StatusOK, resPostMessageSend{Message: "Ok"})
	if err != nil {
		log.Print(err)
	}
}

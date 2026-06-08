package routes

import (
	"botDashboard/internal/command"
	"botDashboard/internal/model"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type bodyPostBotRestart struct {
	Service string `json:"service"`
}

type resBotRestart struct {
	Message string `json:"message"`
}

type serviceControlAction struct {
	command func(serviceName string) command.Command
	audit   string
	summary string
}

func postBotServiceAction(ctx *silverlining.Context, body []byte, action serviceControlAction) {
	var req bodyPostBotRestart
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if req.Service == "" {
		GetError(ctx, &Error{Message: "Service is required", Status: http.StatusBadRequest})
		return
	}

	serviceName := command.DisplayServiceName(req.Service)
	text := action.command(req.Service).Execute()
	recordAdminAudit(ctx, action.audit, serviceName, action.summary+" сервис "+serviceName, nil)

	err = ctx.WriteJSON(http.StatusOK, resBotRestart{Message: text})
	if err != nil {
		log.Print(err)
	}
}

func PostBotRestart(ctx *silverlining.Context, body []byte) {
	postBotServiceAction(ctx, body, serviceControlAction{
		command: func(serviceName string) command.Command {
			return &command.Restart{ServiceName: serviceName}
		},
		audit:   model.AuditActionServiceRestart,
		summary: "Перезапущен",
	})
}

func PostBotStart(ctx *silverlining.Context, body []byte) {
	postBotServiceAction(ctx, body, serviceControlAction{
		command: func(serviceName string) command.Command {
			return &command.Start{ServiceName: serviceName}
		},
		audit:   model.AuditActionServiceStart,
		summary: "Запущен",
	})
}

func PostBotStop(ctx *silverlining.Context, body []byte) {
	postBotServiceAction(ctx, body, serviceControlAction{
		command: func(serviceName string) command.Command {
			return &command.Stop{ServiceName: serviceName}
		},
		audit:   model.AuditActionServiceStop,
		summary: "Остановлен",
	})
}

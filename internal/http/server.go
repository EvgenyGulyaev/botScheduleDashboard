package http

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/http/routes"
	"botDashboard/pkg/singleton"
	"net/http"
)

import (
	"github.com/go-www/silverlining"
)

type Server struct {
	port string
}

func GetServer(port string) *Server {
	return singleton.GetInstance("server", func() interface{} {
		return &Server{
			port: port,
		}
	}).(*Server)
}

func (s *Server) StartHandle() (err error) {
	err = silverlining.ListenAndServe(s.port, func(ctx *silverlining.Context) {
		updateHeader(ctx)
		path := string(ctx.Path())
		switch ctx.Method() {
		case silverlining.MethodGET:
			handleGet(ctx, &path)
		case silverlining.MethodPOST:
			handlePost(ctx, &path)
		case silverlining.MethodOPTIONS:
			ctx.WriteHeader(http.StatusNoContent)
		}
	})
	return
}

func handleGet(ctx *silverlining.Context, path *string) {
	switch *path {
	case "/bot/status":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.GetBotStatus(c)
		})(ctx)
	case "/social/user":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.GetSocialUser(c)
		})(ctx)
	default:
		routes.NotFound(ctx)
	}
	return
}

func handlePost(ctx *silverlining.Context, path *string) {
	body, err := ctx.Body()
	if err != nil {
		routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	switch *path {
	case "/register":
		routes.PostRegister(ctx, body)
	case "/login":
		routes.PostLogin(ctx, body)
	case "/bot/restart":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.PostBotRestart(c, body)
		})(ctx)
	case "/message/send":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.PostMessageSend(c, body)
		})(ctx)
	case "/message/send-all":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.PostMessageSendAll(c, body)
		})(ctx)
	case "/user/block":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.PostUserBlock(c, body)
		})(ctx)
	default:
		routes.NotFound(ctx)
	}
	return
}

func updateHeader(ctx *silverlining.Context) {
	ctx.ResponseHeaders().Set("Access-Control-Allow-Origin", "*")
	ctx.ResponseHeaders().Set("Access-Control-Allow-Credentials", "true")
	ctx.ResponseHeaders().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	ctx.ResponseHeaders().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

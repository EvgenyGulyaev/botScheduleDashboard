package http

import (
	"botDashboard/internal/http/routes"
	"botDashboard/internal/http/status"
	"botDashboard/pkg/singleton"
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
		path := string(ctx.Path())
		switch ctx.Method() {
		case silverlining.MethodGET:
			handleGet(ctx, &path)
		case silverlining.MethodPOST:
			handlePost(ctx, &path)
		}
	})
	return
}

func handleGet(ctx *silverlining.Context, path *string) {
	switch *path {
	default:
		routes.NotFound(ctx)
	}
	return
}

func handlePost(ctx *silverlining.Context, path *string) {
	body, err := ctx.Body()
	if err != nil {
		routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: status.FAIL})
		return
	}

	switch *path {
	case "/register":
		routes.PostRegister(ctx, body)
	case "/login":
		routes.PostLogin(ctx, body)
	default:
		routes.NotFound(ctx)
	}
	return
}

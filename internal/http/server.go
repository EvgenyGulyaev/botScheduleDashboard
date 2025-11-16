package http

import (
	"botDashboard/internal/http/routes"
	"botDashboard/pkg/middleware"
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
	m := map[string]middleware.Hoc{
		"jwt": middleware.GetJwt(),
	}
	err = silverlining.ListenAndServe(s.port, func(ctx *silverlining.Context) {
		path := string(ctx.Path())
		switch ctx.Method() {
		case silverlining.MethodGET:
			handleGet(ctx, &m, &path)
		case silverlining.MethodPOST:
			handlePost(ctx, &m, &path)
		}
	})
	return
}

func handleGet(ctx *silverlining.Context, m *map[string]middleware.Hoc, path *string) {
	switch *path {
	case "bot/status":
		(*m)["jwt"].Check(func(c *silverlining.Context) {
			routes.GetBotStatus(c, nil)
		})(ctx)
	default:
		routes.NotFound(ctx)
	}
	return
}

func handlePost(ctx *silverlining.Context, m *map[string]middleware.Hoc, path *string) {
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
	case "bot/rerun":
		(*m)["jwt"].Check(func(c *silverlining.Context) {
			routes.PostBotRerun(c, body)
		})(ctx)
	default:
		routes.NotFound(ctx)
	}
	return
}

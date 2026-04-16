package http

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/http/routes"
	"botDashboard/pkg/singleton"
	"net/http"
	"net/url"
	"strings"

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
		HandleRequest(ctx)
	})
	return
}

func HandleRequest(ctx *silverlining.Context) {
	updateHeader(ctx)
	path := string(ctx.Path())
	switch ctx.Method() {
	case silverlining.MethodGET:
		handleGet(ctx, path)
	case silverlining.MethodPOST:
		handlePost(ctx, path)
	case silverlining.MethodPATCH:
		handlePatch(ctx, path)
	case silverlining.MethodDELETE:
		handleDelete(ctx, path)
	case silverlining.MethodOPTIONS:
		ctx.WriteHeader(http.StatusNoContent)
	}
}

func handleGet(ctx *silverlining.Context, path string) {
	switch path {
	case "/bot/status":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.GetBotStatus(c)
		})(ctx)
	case "/social/user":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.GetSocialUser(c)
		})(ctx)
	default:
		if strings.HasPrefix(path, "/chat/") {
			handleChatGet(ctx, path)
			return
		}
		routes.NotFound(ctx)
	}
	return
}

func handlePost(ctx *silverlining.Context, path string) {
	body, err := ctx.Body()
	if err != nil {
		routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	switch path {
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
		if strings.HasPrefix(path, "/chat/") {
			handleChatPost(ctx, path, body)
			return
		}
		routes.NotFound(ctx)
	}
	return
}

func handlePatch(ctx *silverlining.Context, path string) {
	body, err := ctx.Body()
	if err != nil {
		routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if strings.HasPrefix(path, "/chat/") {
		handleChatPatch(ctx, path, body)
		return
	}
	routes.NotFound(ctx)
}

func handleDelete(ctx *silverlining.Context, path string) {
	body, err := ctx.Body()
	if err != nil {
		routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if strings.HasPrefix(path, "/chat/") {
		handleChatDelete(ctx, path, body)
		return
	}
	routes.NotFound(ctx)
}

func handleChatGet(ctx *silverlining.Context, path string) {
	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		switch path {
		case "/chat/users":
			routes.GetChatUsers(c)
		case "/chat/conversations":
			routes.GetChatConversations(c)
		default:
			parts := chatPathParts(path)
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" {
				routes.GetChatMessages(c, parts[2])
				return
			}
			routes.NotFound(c)
		}
	})(ctx)
}

func handleChatPost(ctx *silverlining.Context, path string, body []byte) {
	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		switch path {
		case "/chat/conversations/direct":
			routes.PostChatDirect(c, body)
		case "/chat/conversations/group":
			routes.PostChatGroup(c, body)
		default:
			parts := chatPathParts(path)
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "read" {
				routes.PostChatRead(c, parts[2], body)
				return
			}
			if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" && parts[4] == "members" {
				routes.PostChatGroupMembers(c, parts[3], body)
				return
			}
			routes.NotFound(c)
		}
	})(ctx)
}

func handleChatPatch(ctx *silverlining.Context, path string, body []byte) {
	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		parts := chatPathParts(path)
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" {
			routes.PatchChatGroup(c, parts[3], body)
			return
		}
		routes.NotFound(c)
	})(ctx)
}

func handleChatDelete(ctx *silverlining.Context, path string, body []byte) {
	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		parts := chatPathParts(path)
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" {
			routes.DeleteChatGroup(c, parts[3])
			return
		}
		if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" && parts[4] == "members" {
			routes.DeleteChatGroupMembers(c, parts[3], body)
			return
		}
		routes.NotFound(c)
	})(ctx)
}

func chatPathParts(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	rawParts := strings.Split(trimmed, "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			parts = append(parts, part)
			continue
		}
		parts = append(parts, unescaped)
	}
	return parts
}

func updateHeader(ctx *silverlining.Context) {
	ctx.ResponseHeaders().Set("Access-Control-Allow-Origin", "*")
	ctx.ResponseHeaders().Set("Access-Control-Allow-Credentials", "true")
	ctx.ResponseHeaders().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	ctx.ResponseHeaders().Set("Access-Control-Expose-Headers", "X-Auth-Token")
	ctx.ResponseHeaders().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
}

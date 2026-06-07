package http

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/http/routes"
	"botDashboard/pkg/singleton"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-www/silverlining"
)

const defaultServerMaxBodySize int64 = 12 * 1024 * 1024

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
	server := &silverlining.Server{
		MaxBodySize: defaultServerMaxBodySize,
		Handler:     HandleRequest,
	}
	ln, err := net.Listen("tcp", s.port)
	if err != nil {
		return err
	}
	return server.Serve(ln)
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
	case silverlining.MethodPUT:
		handlePut(ctx, path)
	case silverlining.MethodDELETE:
		handleDelete(ctx, path)
	case silverlining.MethodOPTIONS:
		ctx.WriteHeader(http.StatusNoContent)
	}
}

func handleGet(ctx *silverlining.Context, path string) {
	switch path {
	case "/profile":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.GetProfile(c)
		})(ctx)
	case "/alice/accounts":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.GetAliceAccounts(c)
		})(ctx)
	case "/auth/google/config":
		routes.GetGoogleAuthConfig(ctx)
	case "/bot/status":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetBotStatus(c)
		})(ctx)
	case "/server/status":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetServerStatus(c)
		})(ctx)
	case "/server/maintenance/preview":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetServerMaintenancePreview(c)
		})(ctx)
	case "/server/ssh-accesses":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetServerSSHAccesses(c)
		})(ctx)
	case "/proxy/runtime/status":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetProxyRuntimeStatus(c)
		})(ctx)
	case "/proxy/nodes":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetProxyNodes(c)
		})(ctx)
	case "/proxy/pools":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetProxyPools(c)
		})(ctx)
	case "/proxy/users":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetProxyUsers(c)
		})(ctx)
	case "/proxy/routes":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetProxyRoutes(c)
		})(ctx)
	case "/social/user":
		middleware.Use([]string{middleware.Admin}, func(c *silverlining.Context) {
			routes.GetSocialUser(c)
		})(ctx)
	case "/admin/users":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetAdminUsers(c)
		})(ctx)
	case "/admin/audit":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.GetAdminAudit(c)
		})(ctx)
	case "/wedding/public-settings":
		routes.GetWeddingPublicSettings(ctx)
	case "/wedding/rsvps":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.GetWeddingRSVPs(c)
		})(ctx)
	case "/wedding/settings":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.GetWeddingSettings(c)
		})(ctx)
	case "/drawing/images":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.GetDrawingImages(c)
		})(ctx)
	case "/drawing/stamps":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.GetDrawingStamps(c)
		})(ctx)
	default:
		if parts := pathParts(path); len(parts) == 4 && parts[0] == "alice" && parts[1] == "accounts" && parts[3] == "resources" {
			middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
				routes.GetAliceAccountResources(c, parts[2])
			})(ctx)
			return
		}
		if strings.HasPrefix(path, "/chat/") {
			handleChatGet(ctx, path)
			return
		}
		if parts := pathParts(path); len(parts) == 4 && parts[0] == "proxy" && parts[1] == "users" && parts[3] == "vless-link" {
			id, err := url.PathUnescape(parts[2])
			if err != nil {
				routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
				return
			}
			middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
				routes.GetProxyUserVlessLink(c, id)
			})(ctx)
			return
		}
		if parts := pathParts(path); len(parts) == 3 && parts[0] == "drawing" && parts[1] == "images" {
			id, err := url.PathUnescape(parts[2])
			if err != nil {
				routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
				return
			}
			middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
				routes.GetDrawingImage(c, id)
			})(ctx)
			return
		}
		if parts := pathParts(path); len(parts) == 4 && parts[0] == "drawing" && parts[1] == "images" && parts[3] == "content" {
			id, err := url.PathUnescape(parts[2])
			if err != nil {
				routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
				return
			}
			middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
				routes.GetDrawingImageContent(c, id)
			})(ctx)
			return
		}
		if parts := pathParts(path); len(parts) == 3 && parts[0] == "drawing" && parts[1] == "stamps" {
			id, err := url.PathUnescape(parts[2])
			if err != nil {
				routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
				return
			}
			middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
				routes.GetDrawingStamp(c, id)
			})(ctx)
			return
		}
		if parts := pathParts(path); len(parts) == 4 && parts[0] == "drawing" && parts[1] == "stamps" && parts[3] == "content" {
			id, err := url.PathUnescape(parts[2])
			if err != nil {
				routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
				return
			}
			middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
				routes.GetDrawingStampContent(c, id)
			})(ctx)
			return
		}
		routes.NotFound(ctx)
	}
	return
}

func handlePost(ctx *silverlining.Context, path string) {
	if strings.HasPrefix(path, "/chat/") {
		handleChatPost(ctx, path)
		return
	}

	if path == "/drawing/images" {
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PostDrawingImage(c)
		})(ctx)
		return
	}
	if path == "/drawing/stamps" {
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PostDrawingStamp(c)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 4 && parts[0] == "proxy" && parts[1] == "nodes" && parts[3] == "check" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostProxyNodeCheck(c, id)
		})(ctx)
		return
	}

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
	case "/auth/google":
		routes.PostGoogleAuth(ctx, body)
	case "/auth/forgot-password":
		routes.PostForgotPassword(ctx, body)
	case "/auth/reset-password":
		routes.PostResetPassword(ctx, body)
	case "/profile/push-subscriptions":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PostProfilePushSubscriptions(c, body)
		})(ctx)
	case "/alice/announce":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PostAliceAnnounce(c, body)
		})(ctx)
	case "/alice/announce/test":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PostAliceAnnounceTest(c, body)
		})(ctx)
	case "/alice/cleanup-scenarios":
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PostAliceCleanupScenarios(c, body)
		})(ctx)
	case "/bot/restart":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostBotRestart(c, body)
		})(ctx)
	case "/server/maintenance/cleanup":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostServerMaintenanceCleanup(c, body)
		})(ctx)
	case "/server/ssh-accesses":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostServerSSHAccess(c, body)
		})(ctx)
	case "/proxy/runtime/apply":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostProxyRuntimeApply(c)
		})(ctx)
	case "/proxy/nodes":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostProxyNode(c, body)
		})(ctx)
	case "/proxy/pools":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostProxyPool(c, body)
		})(ctx)
	case "/proxy/users":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostProxyUser(c, body)
		})(ctx)
	case "/proxy/routes":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostProxyRoute(c, body)
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
	case "/admin/users":
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PostAdminUser(c, body)
		})(ctx)
	case "/wedding/access/verify":
		routes.PostWeddingAccessVerify(ctx, body)
	case "/wedding/rsvps":
		routes.PostWeddingRSVP(ctx, body)
	default:
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
	if path == "/profile" {
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PatchProfile(c, body)
		})(ctx)
		return
	}
	if path == "/wedding/settings" {
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PatchWeddingSettings(c, body)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "wedding" && parts[1] == "rsvps" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PatchWeddingRSVP(c, id, body)
		})(ctx)
		return
	}
	if strings.HasPrefix(path, "/chat/") {
		handleChatPatch(ctx, path, body)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "proxy" && parts[1] == "nodes" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PatchProxyNode(c, id, body)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "proxy" && parts[1] == "routes" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PatchProxyRoute(c, id, body)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "proxy" && parts[1] == "pools" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PatchProxyPool(c, id, body)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "proxy" && parts[1] == "users" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PatchProxyUser(c, id, body)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "admin" && parts[1] == "users" {
		email, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.PatchAdminUser(c, email, body)
		})(ctx)
		return
	}
	routes.NotFound(ctx)
}

func handleDelete(ctx *silverlining.Context, path string) {
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "drawing" && parts[1] == "images" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.DeleteDrawingImage(c, id)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "drawing" && parts[1] == "stamps" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.DeleteDrawingStamp(c, id)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "server" && parts[1] == "ssh-accesses" {
		username, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.DeleteServerSSHAccess(c, username)
		})(ctx)
		return
	}
	body, err := ctx.Body()
	if err != nil {
		routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if path == "/profile/push-subscriptions" {
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.DeleteProfilePushSubscriptions(c, body)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "wedding" && parts[1] == "rsvps" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.DeleteWeddingRSVP(c, id)
		})(ctx)
		return
	}
	if strings.HasPrefix(path, "/chat/") {
		handleChatDelete(ctx, path, body)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "proxy" && parts[1] == "nodes" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.DeleteProxyNode(c, id)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "proxy" && parts[1] == "routes" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.DeleteProxyRoute(c, id)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "admin" && parts[1] == "users" {
		email, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.SuperAdmin}, func(c *silverlining.Context) {
			routes.DeleteAdminUser(c, email)
		})(ctx)
		return
	}
	routes.NotFound(ctx)
}

func handlePut(ctx *silverlining.Context, path string) {
	if strings.HasPrefix(path, "/chat/") {
		body, err := ctx.Body()
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		handleChatPut(ctx, path, body)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "drawing" && parts[1] == "images" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PutDrawingImage(c, id)
		})(ctx)
		return
	}
	if parts := pathParts(path); len(parts) == 3 && parts[0] == "drawing" && parts[1] == "stamps" {
		id, err := url.PathUnescape(parts[2])
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
			routes.PutDrawingStamp(c, id)
		})(ctx)
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
		case "/chat/calls/config":
			routes.GetChatCallConfig(c)
		case "/chat/search":
			routes.GetChatSearch(c)
		case "/chat/favorites":
			routes.GetChatFavorites(c)
		case "/chat/reminders":
			routes.GetChatReminders(c)
		default:
			parts := chatPathParts(path)
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" {
				routes.GetChatMessages(c, parts[2])
				return
			}
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "call" {
				routes.GetChatCall(c, parts[2])
				return
			}
			if len(parts) == 3 && parts[0] == "chat" && parts[1] == "drafts" {
				routes.GetChatDraft(c, parts[2])
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "audio" {
				routes.GetChatAudio(c, parts[2], parts[4])
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "image" {
				routes.GetChatImage(c, parts[2], parts[4])
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "file" {
				routes.GetChatFile(c, parts[2], parts[4])
				return
			}
			routes.NotFound(c)
		}
	})(ctx)
}

func pathParts(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	return parts
}

func handleChatPost(ctx *silverlining.Context, path string) {
	parts := chatPathParts(path)
	if path == "/chat/system-notifications" {
		body, err := ctx.Body()
		if err != nil {
			routes.GetError(ctx, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
		routes.PostChatSystemNotification(ctx, body)
		return
	}
	if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "audio" {
		routes.PostChatAudioWithToken(ctx, parts[2], parts[4])
		return
	}
	if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "image" {
		routes.PostChatImageWithToken(ctx, parts[2], parts[4])
		return
	}
	if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "file" {
		routes.PostChatFileWithToken(ctx, parts[2], parts[4])
		return
	}

	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		readBody := func() ([]byte, string, bool) {
			contentType, _ := c.RequestHeaders().Get("Content-Type")
			body, err := c.Body()
			if err != nil {
				routes.GetError(c, &routes.Error{Message: err.Error(), Status: http.StatusBadRequest})
				return nil, "", false
			}
			return body, contentType, true
		}

		switch path {
		case "/chat/conversations/direct":
			body, _, ok := readBody()
			if !ok {
				return
			}
			routes.PostChatDirect(c, body)
		case "/chat/conversations/group":
			body, _, ok := readBody()
			if !ok {
				return
			}
			routes.PostChatGroup(c, body)
		default:
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "read" {
				body, _, ok := readBody()
				if !ok {
					return
				}
				routes.PostChatRead(c, parts[2], body)
				return
			}
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "calls" {
				routes.PostChatCallStart(c, parts[2])
				return
			}
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "audio" {
				routes.PostChatAudio(c, parts[2])
				return
			}
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "image" {
				routes.PostChatImage(c, parts[2])
				return
			}
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "file" {
				routes.PostChatFile(c, parts[2])
				return
			}
			if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "forward" {
				body, _, ok := readBody()
				if !ok {
					return
				}
				routes.PostChatForward(c, parts[2], body)
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "calls" && parts[5] == "join" {
				routes.PostChatCallJoin(c, parts[2], parts[4])
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "calls" && parts[5] == "leave" {
				routes.PostChatCallLeave(c, parts[2], parts[4])
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "calls" && parts[5] == "end" {
				routes.PostChatCallEnd(c, parts[2], parts[4])
				return
			}
			if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "reminders" {
				body, _, ok := readBody()
				if !ok {
					return
				}
				routes.PostChatReminder(c, parts[2], parts[4], body)
				return
			}
			if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" && parts[4] == "members" {
				body, _, ok := readBody()
				if !ok {
					return
				}
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
		if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" && parts[4] == "members" {
			routes.PatchChatGroupMember(c, parts[3], parts[5], body)
			return
		}
		if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" {
			routes.PatchChatMessage(c, parts[2], parts[4], body)
			return
		}
		routes.NotFound(c)
	})(ctx)
}

func handleChatDelete(ctx *silverlining.Context, path string, body []byte) {
	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		parts := chatPathParts(path)
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "pin" {
			routes.DeleteChatPin(c, parts[2])
			return
		}
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" {
			routes.DeleteChatGroup(c, parts[3])
			return
		}
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "direct" {
			routes.DeleteChatDirect(c, parts[3])
			return
		}
		if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[2] == "group" && parts[4] == "members" {
			routes.DeleteChatGroupMembers(c, parts[3], body)
			return
		}
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" {
			routes.DeleteChatMessages(c, parts[2])
			return
		}
		if len(parts) == 5 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" {
			routes.DeleteChatMessage(c, parts[2], parts[4])
			return
		}
		if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "reaction" {
			routes.DeleteChatReaction(c, parts[2], parts[4])
			return
		}
		if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "favorite" {
			routes.DeleteChatFavorite(c, parts[2], parts[4])
			return
		}
		if len(parts) == 3 && parts[0] == "chat" && parts[1] == "drafts" {
			routes.DeleteChatDraft(c, parts[2])
			return
		}
		if len(parts) == 3 && parts[0] == "chat" && parts[1] == "reminders" {
			routes.DeleteChatReminder(c, parts[2])
			return
		}
		routes.NotFound(c)
	})(ctx)
}

func handleChatPut(ctx *silverlining.Context, path string, body []byte) {
	middleware.Use([]string{middleware.Auth}, func(c *silverlining.Context) {
		parts := chatPathParts(path)
		if len(parts) == 4 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "pin" {
			routes.PutChatPin(c, parts[2], body)
			return
		}
		if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "calls" && parts[5] == "mute" {
			routes.PutChatCallMute(c, parts[2], parts[4], body)
			return
		}
		if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "reaction" {
			routes.PutChatReaction(c, parts[2], parts[4], body)
			return
		}
		if len(parts) == 6 && parts[0] == "chat" && parts[1] == "conversations" && parts[3] == "messages" && parts[5] == "favorite" {
			routes.PutChatFavorite(c, parts[2], parts[4])
			return
		}
		if len(parts) == 3 && parts[0] == "chat" && parts[1] == "drafts" {
			routes.PutChatDraft(c, parts[2], body)
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
	ctx.ResponseHeaders().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
}

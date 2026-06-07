package routes

import (
	"botDashboard/internal/proxy"
	"context"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetProxyRuntimeStatus(ctx *silverlining.Context) {
	writeProxyResponse(ctx, http.MethodGet, "/runtime/status", nil)
}

func PostProxyRuntimeApply(ctx *silverlining.Context) {
	writeProxyResponse(ctx, http.MethodPost, "/runtime/apply", nil)
}

func GetProxyNodes(ctx *silverlining.Context) {
	writeProxyResponse(ctx, http.MethodGet, "/nodes", nil)
}

func PostProxyNode(ctx *silverlining.Context, body []byte) {
	writeProxyResponse(ctx, http.MethodPost, "/nodes", body)
}

func PatchProxyNode(ctx *silverlining.Context, id string, body []byte) {
	writeProxyResponse(ctx, http.MethodPatch, "/nodes/"+id, body)
}

func DeleteProxyNode(ctx *silverlining.Context, id string) {
	writeProxyResponse(ctx, http.MethodDelete, "/nodes/"+id, nil)
}

func GetProxyPools(ctx *silverlining.Context) {
	writeProxyResponse(ctx, http.MethodGet, "/pools", nil)
}

func PostProxyPool(ctx *silverlining.Context, body []byte) {
	writeProxyResponse(ctx, http.MethodPost, "/pools", body)
}

func PatchProxyPool(ctx *silverlining.Context, id string, body []byte) {
	writeProxyResponse(ctx, http.MethodPatch, "/pools/"+id, body)
}

func GetProxyUsers(ctx *silverlining.Context) {
	writeProxyResponse(ctx, http.MethodGet, "/users", nil)
}

func PostProxyUser(ctx *silverlining.Context, body []byte) {
	writeProxyResponse(ctx, http.MethodPost, "/users", body)
}

func PatchProxyUser(ctx *silverlining.Context, id string, body []byte) {
	writeProxyResponse(ctx, http.MethodPatch, "/users/"+id, body)
}

func GetProxyUserVlessLink(ctx *silverlining.Context, id string) {
	writeProxyResponse(ctx, http.MethodGet, "/users/"+id+"/vless-link", nil)
}

func writeProxyResponse(ctx *silverlining.Context, method, path string, body []byte) {
	response, err := proxy.NewClientFromEnv().Do(context.Background(), method, path, body)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
		return
	}
	if err := ctx.WriteJSON(response.Status, response.Data); err != nil {
		log.Print(err)
	}
}

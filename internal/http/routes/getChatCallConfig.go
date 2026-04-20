package routes

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-www/silverlining"
)

type chatIceServerDTO struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type chatCallConfigDTO struct {
	IceServers []chatIceServerDTO `json:"ice_servers"`
}

func GetChatCallConfig(ctx *silverlining.Context) {
	if _, err := currentChatUser(ctx); err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	response := chatCallConfigDTO{
		IceServers: buildWebRTCIceServers(),
	}
	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}

func buildWebRTCIceServers() []chatIceServerDTO {
	stunURLs := splitEnvList(os.Getenv("CHAT_WEBRTC_STUN_URLS"))
	turnURLs := splitEnvList(os.Getenv("CHAT_WEBRTC_TURN_URLS"))
	servers := make([]chatIceServerDTO, 0, 2)

	if len(stunURLs) > 0 {
		servers = append(servers, chatIceServerDTO{URLs: stunURLs})
	}
	if len(turnURLs) > 0 {
		servers = append(servers, chatIceServerDTO{
			URLs:       turnURLs,
			Username:   strings.TrimSpace(os.Getenv("CHAT_WEBRTC_TURN_USERNAME")),
			Credential: strings.TrimSpace(os.Getenv("CHAT_WEBRTC_TURN_CREDENTIAL")),
		})
	}
	if len(servers) == 0 {
		servers = append(servers, chatIceServerDTO{
			URLs: []string{"stun:stun.l.google.com:19302"},
		})
	}
	return servers
}

func splitEnvList(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

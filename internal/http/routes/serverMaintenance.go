package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"botDashboard/internal/system"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type serverMaintenanceCleanupBody struct {
	Items []string `json:"items"`
}

func GetServerMaintenancePreview(ctx *silverlining.Context) {
	if err := ctx.WriteJSON(http.StatusOK, system.CollectMaintenancePlan(serverMaintenanceOptions())); err != nil {
		logChatError(err)
	}
}

func PostServerMaintenanceCleanup(ctx *silverlining.Context, body []byte) {
	var req serverMaintenanceCleanupBody
	if err := json.Unmarshal(body, &req); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	if containsMaintenanceItem(req.Items, "chat_media_old") {
		repo := store.GetChatRepository()
		if _, err := repo.CleanupExpiredMediaMessages(); err != nil {
			GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
			return
		}
	}

	result, err := system.RunMaintenanceCleanup(req.Items, serverMaintenanceOptions())
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	if len(req.Items) > 0 {
		recordAdminAudit(ctx, model.AuditActionServerMaintenance, "server", "Выполнена очистка сервера", map[string]string{
			"items":   strings.Join(req.Items, ","),
			"cleaned": result.Cleaned,
		})
	}

	if err := ctx.WriteJSON(http.StatusOK, result); err != nil {
		logChatError(err)
	}
}

func serverMaintenanceOptions() system.MaintenanceOptions {
	options := system.DefaultMaintenanceOptions()
	options.ChatMediaPaths = []string{
		store.CHAT_AUDIO_DIR,
		store.CHAT_IMAGE_DIR,
		store.CHAT_FILE_DIR,
	}
	return options
}

func containsMaintenanceItem(items []string, key string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == key {
			return true
		}
	}
	return false
}

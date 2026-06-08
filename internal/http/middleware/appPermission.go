package middleware

import (
	"botDashboard/internal/config"
	"botDashboard/internal/model"

	"github.com/go-www/silverlining"
)

func RequireAppPermission(app string, finalHandler func(c *silverlining.Context)) func(c *silverlining.Context) {
	c := config.LoadConfig()
	if c.Env["MIDDLEWARE_OFF"] == "true" {
		return finalHandler
	}

	return func(ctx *silverlining.Context) {
		user, err := GetAuth().getUserByToken(ctx)
		if err != nil {
			handleError(ctx, err.Error())
			return
		}
		if user.IsSuperAdmin || model.AppAllowed(app, user.AppPermissions) {
			finalHandler(ctx)
			return
		}
		handleError(ctx, "User has no access to "+app)
	}
}

func RequireProxyAccess(finalHandler func(c *silverlining.Context)) func(c *silverlining.Context) {
	return RequireAppPermission(model.DefaultAppProxy, finalHandler)
}

package command

var serviceAliases = map[string]string{
	"alice-TTS":             "alice-speaker-service",
	"alice-speaker-service": "alice-speaker-service",
	"bot":                   "bot",
	"bot-discord":           "bot-nickname",
	"bot-nickname":          "bot-nickname",
	"chat":                  "dashboard-chat",
	"dashboard-chat":        "dashboard-chat",
	"dashboard":             "dashboard",
	"drawyer":               "drawing-service",
	"drawing-service":       "drawing-service",
	"geo3d":                 "geo3d",
	"lawyer":                "lawyer-backend",
	"lawyer-backend":        "lawyer-backend",
	"proxy":                 "vpn-gateway",
	"shotener":              "shotener",
	"vpn-gateway":           "vpn-gateway",
}

var serviceDisplayNames = map[string]string{
	"alice-speaker-service": "alice-TTS",
	"bot":                   "bot",
	"bot-nickname":          "bot-discord",
	"dashboard":             "dashboard",
	"dashboard-chat":        "chat",
	"drawing-service":       "drawyer",
	"geo3d":                 "geo3d",
	"lawyer-backend":        "lawyer",
	"shotener":              "shotener",
	"vpn-gateway":           "proxy",
}

func ResolveServiceName(name string) string {
	if unit, ok := serviceAliases[name]; ok {
		return unit
	}
	return name
}

func DisplayServiceName(name string) string {
	unit := ResolveServiceName(name)
	if displayName, ok := serviceDisplayNames[unit]; ok {
		return displayName
	}
	return name
}

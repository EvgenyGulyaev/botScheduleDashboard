package command

import "testing"

func TestResolveServiceNameAliasesDashboardNamesToUnits(t *testing.T) {
	cases := map[string]string{
		"alice-TTS":   "alice-speaker-service",
		"bot-discord": "bot-nickname",
		"chat":        "dashboard-chat",
		"drawyer":     "drawing-service",
		"lawyer":      "lawyer-backend",
		"proxy":       "vpn-gateway",
	}

	for input, want := range cases {
		if got := ResolveServiceName(input); got != want {
			t.Fatalf("ResolveServiceName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDisplayServiceNameReturnsDashboardName(t *testing.T) {
	cases := map[string]string{
		"alice-speaker-service": "alice-TTS",
		"bot-nickname":          "bot-discord",
		"dashboard-chat":        "chat",
		"drawing-service":       "drawyer",
		"lawyer-backend":        "lawyer",
		"vpn-gateway":           "proxy",
	}

	for input, want := range cases {
		if got := DisplayServiceName(input); got != want {
			t.Fatalf("DisplayServiceName(%q) = %q, want %q", input, got, want)
		}
	}
}

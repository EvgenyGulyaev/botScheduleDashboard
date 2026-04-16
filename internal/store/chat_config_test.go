package store

import "testing"

func TestConfigureChatMaxMessagesUsesConfiguredValue(t *testing.T) {
	previousLimit := CHAT_MAX_MESSAGES
	t.Cleanup(func() {
		CHAT_MAX_MESSAGES = previousLimit
	})

	ConfigureChatMaxMessages("250")

	if CHAT_MAX_MESSAGES != 250 {
		t.Fatalf("expected chat max messages to be 250, got %d", CHAT_MAX_MESSAGES)
	}
}

func TestConfigureChatMaxMessagesFallsBackToDefault(t *testing.T) {
	cases := []string{"", "bad", "1", "0", "-10"}

	for _, tc := range cases {
		CHAT_MAX_MESSAGES = 42

		ConfigureChatMaxMessages(tc)

		if CHAT_MAX_MESSAGES != DefaultChatMaxMessages {
			t.Fatalf("expected default for %q, got %d", tc, CHAT_MAX_MESSAGES)
		}
	}
}

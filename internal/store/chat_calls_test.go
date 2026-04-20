package store

import (
	"botDashboard/internal/model"
	"testing"
)

func TestStartJoinMuteAndLeaveCallLifecycle(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
		{Email: "carol@example.com", Login: "carol"},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	call, message, removedIDs, err := repo.StartCall(conv.ID, "alice@example.com", "alice")
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if len(removedIDs) != 0 {
		t.Fatalf("expected no trimmed messages, got %#v", removedIDs)
	}
	if call.ConversationID != conv.ID || call.MessageID != message.ID {
		t.Fatalf("expected call to reference conversation/message, got %#v / %#v", call, message)
	}
	if len(call.Participants) != 1 || call.Participants[0].Email != "alice@example.com" {
		t.Fatalf("expected alice to be first participant, got %#v", call.Participants)
	}
	if message.Type != "call" || message.Call == nil || !message.Call.Joinable {
		t.Fatalf("expected joinable call message, got %#v", message)
	}

	joined, err := repo.JoinCall(conv.ID, call.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("join call: %v", err)
	}
	if len(joined.Participants) != 2 {
		t.Fatalf("expected 2 participants after join, got %#v", joined.Participants)
	}

	muted, err := repo.SetCallMuted(conv.ID, call.ID, "bob@example.com", true)
	if err != nil {
		t.Fatalf("mute call: %v", err)
	}
	if !muted.Participants[1].Muted {
		t.Fatalf("expected bob muted, got %#v", muted.Participants)
	}

	stillActive, ended, updatedMessage, err := repo.LeaveCall(conv.ID, call.ID, "alice@example.com")
	if err != nil {
		t.Fatalf("leave call by alice: %v", err)
	}
	if ended {
		t.Fatalf("expected call to stay active while bob remains")
	}
	if len(stillActive.Participants) != 1 || stillActive.Participants[0].Email != "bob@example.com" {
		t.Fatalf("expected only bob to remain, got %#v", stillActive.Participants)
	}
	if updatedMessage.Call == nil || !updatedMessage.Call.Joinable {
		t.Fatalf("expected message to stay joinable while call active, got %#v", updatedMessage)
	}

	_, ended, updatedMessage, err = repo.LeaveCall(conv.ID, call.ID, "bob@example.com")
	if err != nil {
		t.Fatalf("leave call by bob: %v", err)
	}
	if !ended {
		t.Fatalf("expected call to end after last participant leaves")
	}
	if updatedMessage.Call == nil || updatedMessage.Call.Joinable || updatedMessage.Call.EndedAt == nil {
		t.Fatalf("expected call message to become ended, got %#v", updatedMessage)
	}
	if _, err := repo.GetActiveCall(conv.ID); err == nil {
		t.Fatalf("expected no active call after last participant leaves")
	}
}

func TestCallRestrictionsByUserAndCapacity(t *testing.T) {
	repo := newChatRepo(t)

	first, err := repo.CreateGroupConversation("One", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
		{Email: "carol@example.com", Login: "carol"},
		{Email: "dave@example.com", Login: "dave"},
		{Email: "erin@example.com", Login: "erin"},
	})
	if err != nil {
		t.Fatalf("create first conversation: %v", err)
	}
	second, err := repo.CreateGroupConversation("Two", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "zoe@example.com", Login: "zoe"},
	})
	if err != nil {
		t.Fatalf("create second conversation: %v", err)
	}

	call, _, _, err := repo.StartCall(first.ID, "alice@example.com", "alice")
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	for _, participant := range []model.UserData{
		{Email: "bob@example.com", Login: "bob"},
		{Email: "carol@example.com", Login: "carol"},
		{Email: "dave@example.com", Login: "dave"},
	} {
		if _, err := repo.JoinCall(first.ID, call.ID, participant.Email, participant.Login); err != nil {
			t.Fatalf("join call for %s: %v", participant.Email, err)
		}
	}

	if _, err := repo.JoinCall(first.ID, call.ID, "erin@example.com", "erin"); err == nil {
		t.Fatalf("expected call capacity error for fifth participant")
	}
	if _, _, _, err := repo.StartCall(second.ID, "alice@example.com", "alice"); err == nil {
		t.Fatalf("expected second active call error for same user")
	}
}

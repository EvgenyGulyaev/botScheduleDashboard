package event

const (
	ChatCommandMessageSend       = "chat.command.message.send"
	ChatCommandMessageRead       = "chat.command.message.read"
	ChatCommandMessageDelivered  = "chat.command.message.delivered"
	ChatCommandPresence          = "chat.command.presence"
	ChatCommandTyping            = "chat.command.typing"
	ChatEventMessagePersisted    = "chat.event.message.persisted"
	ChatEventMessageUpdated      = "chat.event.message.updated"
	ChatEventMessageDeleted      = "chat.event.message.deleted"
	ChatEventMessageDelivered    = "chat.event.message.delivered"
	ChatEventMessageReadUpdated  = "chat.event.message.read.updated"
	ChatEventConversationUpdated = "chat.event.conversation.updated"
	ChatEventPresenceUpdated     = "chat.event.presence.updated"
	ChatEventTyping              = "chat.event.typing"
	ChatEventCallStarted         = "chat.event.call.started"
	ChatEventCallUpdated         = "chat.event.call.updated"
	ChatEventCallEnded           = "chat.event.call.ended"
)

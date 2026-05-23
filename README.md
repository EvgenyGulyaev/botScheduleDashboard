# botScheduleDashboard
Api for botSchedule

## System notifications

External systems can create read-only chat notifications for a user through:

```http
POST /chat/system-notifications
Authorization: Bearer <SYSTEM_NOTIFICATIONS_API_TOKEN>
Content-Type: application/json
```

Configure the shared service token in the API environment:

```bash
SYSTEM_NOTIFICATIONS_API_TOKEN=<shared-secret>
```

Request body:

```json
{
  "recipient_email": "user@example.com",
  "title": "Request created",
  "text": "A request was created in an external system.",
  "source": "crm",
  "external_id": "REQ-123",
  "url": "https://example.com/requests/REQ-123",
  "announce_on_alice": true
}
```

`recipient_email` is required. At least one of `title` or `text` is required.
`announce_on_alice` defaults to `true`; send `false` to keep the notification chat-only.

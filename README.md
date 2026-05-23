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
  "title": "Сформировалась заявка по услугам",
  "text": "Имя: Иван\nТелефон: +79990000000",
  "announce_on_alice": true
}
```

`recipient_email` is required. At least one of `title` or `text` is required.
`announce_on_alice` defaults to `true`; send `false` to keep the notification chat-only.
`source` is accepted for compatibility but is not shown in the user-facing chat text.

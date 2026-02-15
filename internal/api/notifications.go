package api

import "aim-chat/go-backend/internal/app"

type NotificationEvent = app.NotificationEvent

type notificationHub = app.NotificationHub

func newNotificationHub(limit int) *notificationHub {
	return app.NewNotificationHub(limit)
}

package app

import (
	"context"
)

type DaemonService interface {
	CoreAPI
	StartNetworking(ctx context.Context) error
	StopNetworking(ctx context.Context) error
	SubscribeNotifications(cursor int64) ([]NotificationEvent, <-chan NotificationEvent, func())
	ListenAddresses() []string
}

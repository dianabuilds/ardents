package contracts

import "aim-chat/go-backend/internal/app"

// CoreAPI is a compatibility alias for gradual migration from internal/app to
// internal/app/contracts without behavior changes.
type CoreAPI = app.CoreAPI
type DaemonService = app.DaemonService

type IdentityDomain = app.IdentityDomain
type SessionDomain = app.SessionDomain
type MessageRepository = app.MessageRepository
type AttachmentRepository = app.AttachmentRepository
type TransportNode = app.TransportNode
type NotificationBus = app.NotificationBus

type ServiceOptions = app.ServiceOptions

type WirePayload = app.WirePayload
type CategorizedError = app.CategorizedError
type DeviceRevocationDeliveryError = app.DeviceRevocationDeliveryError

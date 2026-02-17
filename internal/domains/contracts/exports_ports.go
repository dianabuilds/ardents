package contracts

import contractports "aim-chat/go-backend/internal/domains/contracts/ports"

type CoreAPI = contractports.CoreAPI
type IdentityAPI = contractports.IdentityAPI
type MessagingAPI = contractports.MessagingAPI
type GroupAPI = contractports.GroupAPI
type InboxAPI = contractports.InboxAPI
type PrivacyAPI = contractports.PrivacyAPI
type NetworkAPI = contractports.NetworkAPI
type DaemonService = contractports.DaemonService
type NotificationEvent = contractports.NotificationEvent
type IdentityDomain = contractports.IdentityDomain
type PrivacySettingsStateStore = contractports.PrivacySettingsStateStore
type BlocklistStateStore = contractports.BlocklistStateStore
type CategorizedError = contractports.CategorizedError
type DeviceRevocationDeliveryError = contractports.DeviceRevocationDeliveryError

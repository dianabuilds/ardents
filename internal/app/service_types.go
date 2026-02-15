package app

import (
	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/pkg/models"
)

type CategorizedError struct {
	Category string
	Err      error
}

func (e *CategorizedError) Error() string {
	return e.Err.Error()
}

func (e *CategorizedError) Unwrap() error {
	return e.Err
}

type DeviceRevocationDeliveryError struct {
	Attempted int
	Failed    int
	Failures  map[string]string
}

func (e *DeviceRevocationDeliveryError) Error() string {
	if e == nil {
		return "device revocation delivery failed"
	}
	if e.Attempted <= 0 {
		return "device revocation delivery failed: no recipients"
	}
	if e.Failed >= e.Attempted {
		return "device revocation delivery failed for all recipients"
	}
	return "device revocation delivery partially failed"
}

func (e *DeviceRevocationDeliveryError) IsFullFailure() bool {
	return e != nil && e.Attempted > 0 && e.Failed >= e.Attempted
}

type WirePayload struct {
	Kind       string                   `json:"kind"`
	Envelope   crypto.MessageEnvelope   `json:"envelope"`
	Plain      []byte                   `json:"plain"`
	Card       *models.ContactCard      `json:"card,omitempty"`
	Receipt    *models.MessageReceipt   `json:"receipt,omitempty"`
	Device     *models.Device           `json:"device,omitempty"`
	DeviceSig  []byte                   `json:"device_sig,omitempty"`
	Revocation *models.DeviceRevocation `json:"revocation,omitempty"`
}

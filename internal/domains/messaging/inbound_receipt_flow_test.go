package messaging

import (
	"aim-chat/go-backend/internal/domains/contracts"
	"testing"

	"aim-chat/go-backend/pkg/models"
)

func TestResolveInboundReceiptHandling_NonReceipt(t *testing.T) {
	h := ResolveInboundReceiptHandling(contracts.WirePayload{Kind: "plain"})
	if h.Handled || h.ShouldUpdate {
		t.Fatalf("unexpected handling: %#v", h)
	}
}

func TestResolveInboundReceiptHandling_ReceiptUnsupportedStatus(t *testing.T) {
	h := ResolveInboundReceiptHandling(contracts.WirePayload{
		Kind: "receipt",
		Receipt: &models.MessageReceipt{
			MessageID: "m1",
			Status:    "queued",
		},
	})
	if !h.Handled || h.ShouldUpdate {
		t.Fatalf("unexpected handling: %#v", h)
	}
}

func TestResolveInboundReceiptHandling_ReceiptDelivered(t *testing.T) {
	h := ResolveInboundReceiptHandling(contracts.WirePayload{
		Kind: "receipt",
		Receipt: &models.MessageReceipt{
			MessageID: "m1",
			Status:    "delivered",
		},
	})
	if !h.Handled || !h.ShouldUpdate || h.MessageID != "m1" || h.Status != "delivered" {
		t.Fatalf("unexpected handling: %#v", h)
	}
}

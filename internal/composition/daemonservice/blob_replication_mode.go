package daemonservice

import (
	"errors"
	"strings"
)

type blobReplicationMode string

const (
	blobReplicationModeOnDemand   blobReplicationMode = "on_demand"
	blobReplicationModePinnedOnly blobReplicationMode = "pinned_only"
	blobReplicationModeNone       blobReplicationMode = "none"
)

var errInvalidBlobReplicationMode = errors.New("invalid blob replication mode")

func parseBlobReplicationMode(raw string) (blobReplicationMode, error) {
	switch blobReplicationMode(strings.ToLower(strings.TrimSpace(raw))) {
	case "":
		return blobReplicationModeOnDemand, nil
	case blobReplicationModeOnDemand, blobReplicationModePinnedOnly, blobReplicationModeNone:
		return blobReplicationMode(strings.ToLower(strings.TrimSpace(raw))), nil
	default:
		return "", errInvalidBlobReplicationMode
	}
}

func resolveBlobReplicationModeFromEnv() blobReplicationMode {
	mode, err := parseBlobReplicationMode(envString("AIM_BLOB_REPLICATION_MODE"))
	if err != nil {
		return blobReplicationModeOnDemand
	}
	return mode
}

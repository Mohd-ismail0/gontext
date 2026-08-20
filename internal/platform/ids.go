package platform

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NewEventID returns a time-ordered unique identifier for ledger/outbox events.
func NewEventID() string {
	return newTimedID()
}

// NewResourceID returns a stable logical resource identifier.
func NewResourceID() string {
	return newTimedID()
}

// NewRevisionID returns an immutable revision identifier.
func NewRevisionID() string {
	return newTimedID()
}

// NewArtifactID returns an artifact/derivation identifier.
func NewArtifactID() string {
	return newTimedID()
}

func newTimedID() string {
	id, err := uuid.NewV7()
	if err == nil {
		return id.String()
	}
	// Fallback: UUIDv4 with millisecond time prefix for rough sortability.
	return fmt.Sprintf("%d-%s", time.Now().UTC().UnixMilli(), uuid.NewString())
}

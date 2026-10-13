package saved

import "context"

// MutationCapabilities describes the current actor's permitted actions on an
// existing saved exploration. It is presentation state, not mutation authority.
type MutationCapabilities struct {
	CanEdit    bool
	CanArchive bool
}

// MutationCapabilityReader evaluates presentation capabilities through the
// same current authorization boundary used by lifecycle mutations.
type MutationCapabilityReader interface {
	MutationCapabilities(context.Context, ReadRequest) (MutationCapabilities, error)
}

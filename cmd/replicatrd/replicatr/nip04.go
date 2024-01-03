package replicatr

import (
	"golang.org/x/exp/slices"
)

// RejectKind04Snoopers prevents reading NIP-04 messages from people not
// involved in the conversation.
func RejectKind04Snoopers(ctx Ctx, filter *Filter) (bool, string) {
	// prevent kind-4 events from being returned to unauthed users, only when
	// authentication is a thing
	if !slices.Contains(filter.Kinds, 4) {
		return false, ""
	}
	ws := GetConnection(ctx)
	s := filter.Authors
	r, _ := filter.Tags["p"]
	switch {
	case ws.AuthedPublicKey == "":
		// not authenticated
		return true, "restricted: this relay does not serve kind-4 to " +
			"unauthenticated users, does your client implement NIP-42?"
	case len(s) == 1 && len(r) < 2 && (s[0] == ws.AuthedPublicKey):
		// allowed filter: ws.authed is sole sender (filter specifies one or all
		// r)
		return false, ""
	case len(r) == 1 && len(s) < 2 && (r[0] == ws.AuthedPublicKey):
		// allowed filter: ws.authed is sole receiver (filter specifies one or
		// all senders)
		return false, ""
	default:
		// restricted filter: do not return any events, even if other elements
		// in filters array were not restricted). client should know better.
		return true, "restricted: authenticated user does not have " +
			"authorization for requested filters."
	}
}

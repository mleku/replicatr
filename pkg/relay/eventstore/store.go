package eventstore

import (
	"context"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/nip1"
)

// Store is a persistence layer for nostr events handled by a relay.
type Store interface {
	// Init is called at the very beginning by [Server.Start], after [Relay.Init],
	// allowing a storage to initialize its internal resources.
	Init() error

	// Close must be called after you're done using the store, to free up resources and so on.
	Close()

	// QueryEvents is invoked upon a client's REQ as described in NIP-01.
	// it should return a channel with the events as they're recovered from a database.
	// the channel should be closed after the events are all delivered.
	QueryEvents(context.Context, *nip1.Filter) (chan *nip1.Event, error)
	// DeleteEvent is used to handle deletion events, as per NIP-09.
	DeleteEvent(context.Context, *nip1.Event) error
	// SaveEvent is called once Relay.AcceptEvent reports true.
	SaveEvent(context.Context, *nip1.Event) error
}
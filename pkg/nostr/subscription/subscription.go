package subscription

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/closeenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/countenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/eventenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/reqenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filters"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/interfaces/relay"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/interfaces/subscriptionoption"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/subscriptionid"
	"mleku.online/git/slog"
)

var log = slog.GetStd()

type T struct {
	Label   string
	Counter int

	Relay   relay.I
	Filters filters.T

	// for this to be treated as a COUNT and not a REQ this must be set
	CountResult chan int64

	// the Events channel emits all EVENTs that come in a Subscription
	// will be closed when the subscription ends
	Events chan *event.T
	mu     sync.Mutex

	// the EndOfStoredEvents channel gets closed when an EOSE comes for that
	// subscription
	EndOfStoredEvents chan struct{}

	// the ClosedReason channel emits the reason when a CLOSED message is
	// received
	ClosedReason chan string

	// Context will be .Done() when the subscription ends
	Context context.T

	live   atomic.Bool
	eosed  atomic.Bool
	closed atomic.Bool
	Cancel context.F

	// this keeps track of the events we've received before the EOSE that we
	// must dispatch before closing the EndOfStoredEvents channel
	storedwg sync.WaitGroup
}

type EventMessage struct {
	Event eventenvelope.T
	Relay string
}

// WithLabel puts a label on the subscription (it is prepended to the automatic
// id) that is sent to relays.
type WithLabel string

func (_ WithLabel) IsSubscriptionOption() {}

var _ subscriptionoption.I = (WithLabel)("")

// GetID return the Nostr subscription ID as given to the I
// it is a concatenation of the label and a serial number.
func (sub *T) GetID() string {
	return sub.Label + ":" + strconv.Itoa(sub.Counter)
}

func (sub *T) Start() {
	<-sub.Context.Done()
	// the subscription ends once the context is canceled (if not already)
	sub.Unsub() // this will set sub.live to false

	// do this so we don't have the possibility of closing the Events channel
	// and then trying to send to it
	sub.mu.Lock()
	close(sub.Events)
	sub.mu.Unlock()
}

func (sub *T) DispatchEvent(evt *event.T) {
	log.T.Ln("dispatching event to channel")
	added := false
	if !sub.eosed.Load() {
		sub.storedwg.Add(1)
		added = true
	}
	go func() {
		sub.mu.Lock()
		defer sub.mu.Unlock()
		if sub.live.Load() {
			select {
			case sub.Events <- evt:
			case <-sub.Context.Done():
			}
		}
		if added {
			sub.storedwg.Done()
		}
	}()
}

func (sub *T) DispatchEose() {
	if sub.eosed.CompareAndSwap(false, true) {
		go func() {
			sub.storedwg.Wait()
			sub.EndOfStoredEvents <- struct{}{}
		}()
	}
}

func (sub *T) DispatchClosed(reason string) {
	if sub.closed.CompareAndSwap(false, true) {
		go func() {
			sub.ClosedReason <- reason
		}()
	}
}

// Unsub closes the subscription, sending "CLOSE" to relay as in NIP-01.
// Unsub() also closes the channel sub.Events and makes a new one.
func (sub *T) Unsub() {
	// cancel the context (if it's not canceled already)
	sub.Cancel()

	// mark subscription as closed and send a CLOSE to the relay (naïve
	// sync.Once implementation)
	if sub.live.CompareAndSwap(true, false) {
		sub.Close()
	}

	// remove subscription from our map
	sub.Relay.Delete(sub.GetID())
}

// Close just sends a CLOSE message. You probably want Unsub() instead.
func (sub *T) Close() {
	if sub.Relay.IsConnected() {
		id := sub.GetID()
		closeMsg := closeenvelope.New(subscriptionid.T(id))
		closeb, _ := closeMsg.MarshalJSON()
		log.D.F("{%s} sending %v", sub.Relay.URL(), string(closeb))
		<-sub.Relay.Write(closeb)
	}
}

// Sub sets sub.T and then calls sub.Fire(ctx).
// The subscription will be closed if the context expires.
func (sub *T) Sub(_ context.T, filters filters.T) {
	sub.Filters = filters
	log.Fail(sub.Fire())
}

// Fire sends the "REQ" command to the relay.
func (sub *T) Fire() error {
	id := sub.GetID()

	var reqb []byte
	if sub.CountResult == nil {
		reqb, _ = (&reqenvelope.T{
			SubscriptionID: subscriptionid.T(id),
			Filters:        sub.Filters,
		}).MarshalJSON()
	} else {
		reqb, _ = (&countenvelope.Request{
			ID:      subscriptionid.T(id),
			Filters: sub.Filters,
		}).MarshalJSON()
	}
	log.T.F("{%s} sending %v", sub.Relay.URL(), string(reqb))

	sub.live.Store(true)
	if err := <-sub.Relay.Write(reqb); err != nil {
		sub.Cancel()
		return fmt.Errorf("failed to write: %w", err)
	}

	return nil
}

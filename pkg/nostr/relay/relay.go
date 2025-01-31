package relay

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/connection"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/authenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/closedenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/countenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/eoseenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/eventenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/noticeenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/okenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filter"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filters"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/interfaces/enveloper"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/interfaces/subscriptionoption"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kind"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/normalize"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/subscription"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/tag"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/tags"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/timestamp"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/puzpuzpuz/xsync/v2"
	"mleku.online/git/slog"
)

var log = slog.GetStd()

type Status int

var subscriptionIDCounter atomic.Int32

type T struct {
	closeMutex sync.Mutex

	url           string
	RequestHeader http.Header // e.g. for origin header

	Connection    *connection.C
	Subscriptions *xsync.MapOf[string, *subscription.T]

	ConnectionError         error
	connectionContext       context.T // will be canceled when the connection closes
	connectionContextCancel context.F

	challenge                     string      // NIP-42 challenge, we only keep the last
	notices                       chan string // NIP-01 NOTICEs
	okCallbacks                   *xsync.MapOf[string, func(bool, string)]
	writeQueue                    chan writeRequest
	subscriptionChannelCloseQueue chan *subscription.T

	// custom things that aren't often used
	//
	AssumeValid bool // this will skip verifying signatures for events received from this relay
}

func (r *T) URL() string { return r.url }

func (r *T) Delete(key string) { r.Subscriptions.Delete(key) }

type writeRequest struct {
	msg    []byte
	answer chan error
}

// NewRelay returns a new relay. The relay connection will be closed when the
// context is canceled.
func NewRelay(c context.T, url string, opts ...Option) *T {
	ctx, cancel := context.Cancel(c)
	r := &T{
		url:                           normalize.URL(url),
		connectionContext:             ctx,
		connectionContextCancel:       cancel,
		Subscriptions:                 xsync.NewMapOf[*subscription.T](),
		okCallbacks:                   xsync.NewMapOf[func(bool, string)](),
		writeQueue:                    make(chan writeRequest),
		subscriptionChannelCloseQueue: make(chan *subscription.T),
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case WithNoticeHandler:
			r.notices = make(chan string)
			go func() {
				for n := range r.notices {
					o(n)
				}
			}()
		}
	}

	return r
}

// Connect returns a relay object connected to url. Once successfully
// connected, cancelling ctx has no effect. To close the connection, call
// r.Close().
func Connect(c context.T, url string, opts ...Option) (*T, error) {
	r := NewRelay(context.Bg(), url, opts...)
	err := r.Connect(c)
	return r, err
}

// When instantiating relay connections, some options may be passed.

// Option is the type of the argument passed for that.
type Option interface {
	IsRelayOption()
}

// WithNoticeHandler just takes notices and is expected to do something with
// them. when not given, defaults to logging the notices.
type WithNoticeHandler func(notice string)

func (_ WithNoticeHandler) IsRelayOption() {}

var _ Option = (WithNoticeHandler)(nil)

// String just returns the relay URL.
func (r *T) String() string {
	return r.url
}

// Context retrieves the context that is associated with this relay connection.
func (r *T) Context() context.T { return r.connectionContext }

// IsConnected returns true if the connection to this relay seems to be active.
func (r *T) IsConnected() bool { return r.connectionContext.Err() == nil }

// Connect tries to establish a websocket connection to r.URL. If the context
// expires before the connection is complete, an error is returned. Once
// successfully connected, context expiration has no effect: call r.Close to
// close the connection.
//
// The underlying relay connection will use a background context. If you want to
// pass a custom context to the underlying relay connection, use NewRelay() and
// then Relay.Connect().
func (r *T) Connect(c context.T) (err error) {
	if r.connectionContext == nil || r.Subscriptions == nil {
		return fmt.Errorf("relay must be initialized with a call to NewRelay()")
	}

	if r.url == "" {
		return fmt.Errorf("invalid relay URL '%s'", r.URL())
	}

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		c, cancel = context.Timeout(c, 7*time.Second)
		defer cancel()
	}
	var conn *connection.C
	conn, err = connection.NewConnection(c, r.url, r.RequestHeader)
	if err != nil {
		return fmt.Errorf("error opening websocket to '%s': %w", r.URL(), err)
	}
	r.Connection = conn

	// ping every 29 seconds
	ticker := time.NewTicker(29 * time.Second)

	// to be used when the connection is closed
	go func() {
		<-r.connectionContext.Done()
		// close these things when the connection is closed
		if r.notices != nil {
			close(r.notices)
		}
		// stop the ticker
		ticker.Stop()
		// close all subscriptions
		r.Subscriptions.Range(func(_ string, sub *subscription.T) bool {
			go sub.Unsub()
			return true
		})
	}()

	// queue all write operations here so we don't do mutex spaghetti
	go func() {
		var err error
		for {
			select {
			case <-ticker.C:
				err = wsutil.WriteClientMessage(r.Connection.Conn, ws.OpPing, nil)
				if err != nil {
					log.D.F("{%s} error writing ping: %v; closing websocket",
						r.URL(), err)
					log.Fail(r.Close()) // this should trigger a context cancelation
					return
				}
			case wr := <-r.writeQueue:
				// all write requests will go through this to prevent races
				if err = r.Connection.WriteMessage(wr.msg); err != nil {
					wr.answer <- err
				}
				close(wr.answer)
			case <-r.connectionContext.Done():
				// stop here
				return
			}
		}
	}()

	// general message reader loop
	go r.MessageReadLoop(conn)
	return nil
}

func (r *T) MessageReadLoop(conn *connection.C) {
	buf := new(bytes.Buffer)
	var err error
	for {
		buf.Reset()
		if err = conn.ReadMessage(r.connectionContext, buf); err != nil {
			r.ConnectionError = err
			log.Fail(r.Close())
			break
		}

		message := buf.Bytes()
		log.T.F("{%s} received %v", r.URL(), string(message))
		var envelope enveloper.I
		envelope, _, err = envelopes.ProcessEnvelope(message)
		if envelope == nil {
			continue
		}

		switch env := envelope.(type) {
		case *noticeenvelope.T:
			// see WithNoticeHandler
			if r.notices != nil {
				r.notices <- env.Text
			} else {
				log.D.F("NOTICE from %s: '%s'", r.URL(), env.Text)
			}
		case *authenvelope.Challenge:
			// if env.Challenge == nil {
			// 	continue
			// }
			r.challenge = env.Challenge
		case *eventenvelope.T:
			if env.SubscriptionID == "" {
				continue
			}
			if s, ok := r.Subscriptions.Load(env.SubscriptionID.String()); !ok {
				log.D.F("{%s} no subscription with id '%s'",
					r.URL(), env.SubscriptionID.String())
				continue
			} else {
				// check if the event matches the desired filter, ignore otherwise
				if !s.Filters.Match(env.Event) {
					log.D.F("{%s} filter does not match: %v ~ %v",
						r.URL(), s.Filters, env.Event)
					continue
				}
				// check signature, ignore invalid, except from trusted (AssumeValid) relays
				if !r.AssumeValid {
					if ok, err = env.Event.CheckSignature(); !ok {
						errmsg := ""
						if log.Fail(err) {
							errmsg = err.Error()
						}
						log.D.F("{%s} bad signature on %s; %s",
							r.URL(), env.Event.ID, errmsg)
						continue
					}
				}
				// dispatch this to the internal .events channel of the
				// subscription
				s.DispatchEvent(env.Event)
			}
		case *eoseenvelope.T:
			if s, ok := r.Subscriptions.Load(env.String()); ok {
				s.DispatchEose()
			}
		case *closedenvelope.T:
			if s, ok := r.Subscriptions.Load(env.ID.String()); ok {
				s.DispatchClosed(env.Reason)
			}
		case *countenvelope.Response:
			if s, ok := r.Subscriptions.Load(env.ID.String()); ok &&
				s.CountResult != nil {

				s.CountResult <- env.Count
			}
		case *okenvelope.T:
			if okCallback, exist := r.okCallbacks.Load(env.ID.String()); exist {
				okCallback(env.OK, env.Reason)
			} else {
				log.D.F("{%s} got an unexpected OK message for event %s",
					r.URL(), env.ID)
			}
		}
	}
}

// Write queues a message to be sent to the relay.
func (r *T) Write(msg []byte) <-chan error {
	ch := make(chan error)
	select {
	case r.writeQueue <- writeRequest{msg: msg, answer: ch}:
	case <-r.connectionContext.Done():
		go func() { ch <- fmt.Errorf("connection closed") }()
	}
	return ch
}

// Publish sends an "EVENT" command to the relay r as in NIP-01 and waits for an
// OK response.
func (r *T) Publish(c context.T, ev *event.T) error {
	return r.publish(c, ev.ID.String(), &eventenvelope.T{Event: ev})
}

// Auth sends an "AUTH" command client->relay as in NIP-42 and waits for an OK
// response.
func (r *T) Auth(c context.T, sign func(ev *event.T) error) error {
	authEvent := &event.T{
		CreatedAt: timestamp.Now(),
		Kind:      kind.ClientAuthentication,
		Tags: tags.T{
			tag.T{"relay", r.URL()},
			tag.T{"challenge", r.challenge},
		},
		Content: "",
	}
	if err := sign(authEvent); err != nil {
		return fmt.Errorf("error signing auth event: %w", err)
	}

	return r.publish(c, authEvent.ID.String(),
		&authenvelope.Response{Event: authEvent})
}

// publish can be used both for EVENT and for AUTH
func (r *T) publish(c context.T, id string, env enveloper.I) error {
	var err error
	var cancel context.F

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		c, cancel = context.Timeout(c, 7*time.Second)
		defer cancel()
	} else {
		// otherwise make the context cancellable so we can stop everything upon
		// receiving an "OK"
		c, cancel = context.Cancel(c)
		defer cancel()
	}

	// listen for an OK callback
	gotOk := false
	r.okCallbacks.Store(id, func(ok bool, reason string) {
		gotOk = true
		if !ok {
			err = fmt.Errorf("msg: %s", reason)
		}
		cancel()
	})
	defer r.okCallbacks.Delete(id)

	// publish event
	envb, _ := env.MarshalJSON()
	log.D.F("{%s} sending %v", r.URL(), string(envb))
	if err := <-r.Write(envb); err != nil {
		return err
	}

	for {
		select {
		case <-c.Done():
			// this will be called when we get an OK or when the context has been canceled
			if gotOk {
				return err
			}
			return c.Err()
		case <-r.connectionContext.Done():
			// this is caused when we lose connectivity
			return err
		}
	}
}

// Subscribe sends a "REQ" command to the relay r as in NIP-01. Events are
// returned through the channel sub.Events. The subscription is closed when
// context ctx is cancelled ("CLOSE" in NIP-01).
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or
// ensuring their `context.T` will be canceled at some point. Failure to do that
// will result in a huge number of halted goroutines being created.
func (r *T) Subscribe(c context.T, f filters.T,
	opts ...subscriptionoption.I) (*subscription.T, error) {

	sub := r.PrepareSubscription(c, f, opts...)

	if err := sub.Fire(); err != nil {
		return nil, fmt.Errorf("couldn't subscribe to %v at %s: %w", f, r.URL(), err)
	}

	return sub, nil
}

// PrepareSubscription creates a subscription, but doesn't fire it.
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or
// ensuring their `context.T` will be canceled at some point. Failure to do that
// will result in a huge number of halted goroutines being created.
func (r *T) PrepareSubscription(c context.T, f filters.T,
	opts ...subscriptionoption.I) *subscription.T {

	if r.Connection == nil {
		panic(fmt.Errorf("must call .Connect() first before calling .Subscribe()"))
	}

	current := subscriptionIDCounter.Add(1)
	ctx, cancel := context.Cancel(c)

	sub := &subscription.T{
		Relay:             r,
		Context:           ctx,
		Cancel:            cancel,
		Counter:           int(current),
		Events:            make(chan *event.T),
		EndOfStoredEvents: make(chan struct{}),
		ClosedReason:      make(chan string, 1),
		Filters:           f,
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case subscription.WithLabel:
			sub.Label = string(o)
		}
	}

	id := sub.GetID()
	r.Subscriptions.Store(id, sub)

	// start handling events, eose, unsub etc:
	go sub.Start()

	return sub
}

func (r *T) QuerySync(c context.T, f *filter.T,
	opts ...subscriptionoption.I) ([]*event.T, error) {
	sub, err := r.Subscribe(c, filters.T{f}, opts...)
	if err != nil {
		return nil, err
	}

	defer sub.Unsub()

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		c, cancel = context.Timeout(c, 7*time.Second)
		defer cancel()
	}

	var events []*event.T
	for {
		select {
		case evt := <-sub.Events:
			if evt == nil {
				// channel is closed
				return events, nil
			}
			events = append(events, evt)
		case <-sub.EndOfStoredEvents:
			return events, nil
		case <-c.Done():
			return events, nil
		}
	}
}

func (r *T) Count(c context.T, filters filters.T,
	opts ...subscriptionoption.I) (int64, error) {

	sub := r.PrepareSubscription(c, filters, opts...)
	sub.CountResult = make(chan int64)

	if err := sub.Fire(); err != nil {
		return 0, err
	}

	defer sub.Unsub()

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		c, cancel = context.Timeout(c, 7*time.Second)
		defer cancel()
	}

	for {
		select {
		case count := <-sub.CountResult:
			return count, nil
		case <-c.Done():
			return 0, c.Err()
		}
	}
}

func (r *T) Close() error {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()

	if r.connectionContextCancel == nil {
		return fmt.Errorf("relay not connected")
	}

	r.connectionContextCancel()
	r.connectionContextCancel = nil
	return r.Connection.Close()
}

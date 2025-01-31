package replicatr

import (
	"errors"
	"sync"

	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/eventenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/noticeenvelope"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filter"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/normalize"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/subscriptionid"
)

type handleFilterParams struct {
	c    context.T
	id   string
	eose *sync.WaitGroup
	ws   *WebSocket
	f    *filter.T
}

func (rl *Relay) handleFilter(h handleFilterParams) (err error) {

	defer h.eose.Done()
	// overwrite the filter (for example, to eliminate some kinds or that we
	// know we don't support)
	for _, ovw := range rl.OverwriteFilter {
		ovw(h.c, h.f)
	}
	if h.f.Limit < 0 {
		err = errors.New("blocked: filter invalidated")
		rl.E.Chk(err)
		return
	}
	// then check if we'll reject this filter (we apply this after overwriting
	// because we may, for example, remove some things from the incoming filters
	// that we know we don't support, and then if the end result is an empty
	// filter we can just reject it)
	for _, reject := range rl.RejectFilter {
		if rej, msg := reject(h.c, h.f); rej {
			rl.E.Chk(h.ws.WriteEnvelope(&noticeenvelope.T{Text: msg}))
			return errors.New(normalize.OKMessage(msg, "blocked"))
		}
	}
	// run the functions to query events (generally just one,
	// but we might be fetching stuff from multiple places)
	h.eose.Add(len(rl.QueryEvents))
	for _, query := range rl.QueryEvents {
		var ch chan *event.T
		if ch, err = query(h.c, h.f); rl.E.Chk(err) {
			rl.E.Chk(h.ws.WriteEnvelope(&noticeenvelope.T{Text: err.Error()}))
			h.eose.Done()
			continue
		}
		go func(ch chan *event.T) {
			for ev := range ch {
				for _, ovw := range rl.OverwriteResponseEvent {
					ovw(h.c, ev)
				}
				rl.E.Chk(h.ws.WriteEnvelope(&eventenvelope.T{
					SubscriptionID: subscriptionid.T(h.id),
					Event:          ev,
				}))
			}
			h.eose.Done()
		}(ch)
	}
	return nil
}

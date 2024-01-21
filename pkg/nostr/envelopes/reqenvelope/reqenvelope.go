package reqenvelope

import (
	"encoding/json"
	"fmt"

	"github.com/Hubmakerlabs/replicatr/pkg/interfaces/enveloper"
	log2 "github.com/Hubmakerlabs/replicatr/pkg/log"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/labels"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filter"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filters"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/subscriptionid"
	"github.com/Hubmakerlabs/replicatr/pkg/wire/array"
	"github.com/Hubmakerlabs/replicatr/pkg/wire/text"
)

var log = log2.GetStd()

// T is the wrapper for a query to a relay.
type T struct {
	SubscriptionID subscriptionid.T
	filters.T
}

var _ enveloper.I = &T{}

func (E *T) UnmarshalJSON(bytes []byte) error {
	// TODO implement me
	panic("implement me")
}

// Label returns the label enum/type of the envelope. The relevant bytes could
// be retrieved using nip1.List[T]
func (E *T) Label() (l string) { return labels.REQ }

func (E *T) ToArray() (arr array.T) {
	arr = array.T{labels.REQ, E.SubscriptionID}
	for _, f := range E.T {
		arr = append(arr, f.ToObject())
	}
	return
}

func (E *T) String() (s string) { return E.ToArray().String() }

func (E *T) Bytes() (s []byte) { return E.ToArray().Bytes() }

func (E *T) MarshalJSON() ([]byte, error) { return E.Bytes(), nil }

// Unmarshal the envelope.
func (E *T) Unmarshal(buf *text.Buffer) (e error) {
	if E == nil {
		return fmt.Errorf("cannot unmarshal to nil pointer")
	}
	// log.D.F("REQ '%s'", buf.Buf[buf.Pos:])
	// Next, find the comma after the label
	if e = buf.ScanThrough(','); e != nil {
		return
	}
	var which byte
	// T can have one or no subscription IDs, if it is present we want
	// to collect it before looking for the filters.
	which, e = buf.ScanForOneOf(false, '{', '"')
	if which == '"' {
		// Next character we find will be open quotes for the subscription ID.
		if e = buf.ScanThrough('"'); e != nil {
			return
		}
		var sid []byte
		// read the string
		if sid, e = buf.ReadUntil('"'); log.Fail(e) {
			return fmt.Errorf("unterminated quotes in JSON, probably truncated read")
		}
		log.D.F("Subscription ID: '%s'", sid)
		E.SubscriptionID = subscriptionid.T(sid)
	}
	// Next, find the comma (there must be one and at least one object brace
	// after it
	if e = buf.ScanThrough(','); e != nil {
		return
	}
	for {
		// find the opening brace of the event object, usually this is the very
		// next character, we aren't checking for valid whitespace because
		// laziness.
		if e = buf.ScanUntil('{'); e != nil {
			return fmt.Errorf("event not found in event envelope")
		}
		// now we should have an event object next. It has no embedded object so
		// it should end with a close brace. This slice will be wrapped in
		// braces and contain paired brackets, braces and quotes.
		var filterArray []byte
		if filterArray, e = buf.ReadEnclosed(); log.Fail(e) {
			return
		}
		// log.D.F("filter: '%s'", filterArray)
		f := &filter.T{}
		if e = json.Unmarshal(filterArray, f); log.Fail(e) {
			return
		}
		E.T = append(E.T, f)
		// log.D.F("remaining: '%s'", buf.Buf[buf.Pos:])
		which = 0
		if which, e = buf.ScanForOneOf(true, ',', ']'); log.Fail(e) {
			return
		}
		// log.D.F("'%s'", string(which))
		if which == ']' {
			break
		}
	}
	return
}
package sdk

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filters"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kind"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kinds"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/normalize"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/pool"
)

type Relay struct {
	URL    string
	Inbox  bool
	Outbox bool
}

func FetchRelaysForPubkey(c context.T, pool *pool.Simple, pubkey string, relays ...string) (r []Relay) {
	c, cancel := context.Cancel(c)
	defer cancel()
	ch := pool.SubManyEose(c, relays, filters.T{
		{
			Kinds: kinds.T{
				kind.RelayListMetadata,
				kind.FollowList,
			},
			Authors: []string{pubkey},
			Limit:   2,
		},
	}, true)
	r = make([]Relay, 0, 20)
	i := 0
	for ie := range ch {
		switch ie.Event.Kind {
		case kind.RelayListMetadata:
			r = append(r, ParseRelaysFromKind10002(ie.Event)...)
		case kind.FollowList:
			r = append(r, ParseRelaysFromKind3(ie.Event)...)
		}
		i++
		if i >= 2 {
			break
		}
	}
	return
}

func ParseRelaysFromKind10002(evt *event.T) (r []Relay) {
	r = make([]Relay, 0, len(evt.Tags))
	for _, tag := range evt.Tags {
		if u := tag.Value(); u != "" && tag[0] == "r" {
			if !IsValidRelayURL(u) {
				continue
			}
			rl := Relay{
				URL: normalize.URL(u),
			}
			if len(tag) == 2 {
				rl.Inbox = true
				rl.Outbox = true
			} else if tag[2] == "write" {
				rl.Outbox = true
			} else if tag[2] == "read" {
				rl.Inbox = true
			}
			r = append(r, rl)
		}
	}
	return
}

func ParseRelaysFromKind3(evt *event.T) (r []Relay) {
	type Item struct {
		Read  bool `json:"read"`
		Write bool `json:"write"`
	}
	items := make(map[string]Item, 20)
	var err error
	if err = json.Unmarshal([]byte(evt.Content), &items); log.Fail(err) {
		// shouldn't this be fatal?
	}
	r = make([]Relay, len(items))
	i := 0
	for u, item := range items {
		if !IsValidRelayURL(u) {
			continue
		}
		rl := Relay{
			URL: normalize.URL(u),
		}
		if item.Read {
			rl.Inbox = true
		}
		if item.Write {
			rl.Outbox = true
		}
		r = append(r, rl)
		i++
	}
	return r
}

func IsValidRelayURL(u string) bool {
	parsed, err := url.Parse(u)
	if log.Fail(err) {
		return false
	}
	if parsed.Scheme != "wss" && parsed.Scheme != "ws" {
		return false
	}
	if len(strings.Split(parsed.Host, ".")) < 2 {
		return false
	}
	return true
}

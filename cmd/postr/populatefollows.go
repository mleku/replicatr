package main

import (
	"encoding/json"

	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filter"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kind"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kinds"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/relay"
)

func (cfg *C) PopulateFollows(f *[]string, start, end *int) RelayIter {
	return func(c context.T, rl *relay.T) bool {
		log.T.Ln("populating follow list of profile", rl.URL(), *f)
		evs, err := rl.QuerySync(c, &filter.T{
			Kinds:   kinds.T{kind.ProfileMetadata},
			Authors: (*f)[*start:*end], // Use the updated end index
			Limit:   *end - *start,
		})
		log.T.S(rl.URL(), evs)
		if log.Fail(err) {
			return true
		}
		for _, ev := range evs {
			p := &Metadata{}
			err = json.Unmarshal([]byte(ev.Content), p)
			if err == nil {
				cfg.Lock()
				cfg.Follows[ev.PubKey] = p
				cfg.FollowsRelays[ev.PubKey] = append(cfg.FollowsRelays[ev.PubKey],
					rl.URL())
				cfg.Unlock()
			}
		}
		return true
	}
}

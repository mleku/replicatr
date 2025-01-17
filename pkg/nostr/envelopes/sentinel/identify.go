package sentinel

import (
	"fmt"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/labels"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/wire/text"
	"mleku.online/git/slog"
)

var log = slog.GetStd()

// Identify takes a byte slice and scans it as a nostr Envelope array, and
// returns the label type and a text.Buffer that is ready for the Read function
// to generate the appropriate structure.
func Identify(b []byte) (match string, buf *text.Buffer, err error) {
	// The bytes must be valid JSON but we can't assume they are free of
	// whitespace... So we will use some tools.
	buf = text.NewBuffer(b)
	// First there must be an opening bracket.
	if err = buf.ScanThrough('['); log.Fail(err) {
		return
	}
	// Then a quote.
	if err = buf.ScanThrough('"'); log.Fail(err) {
		return
	}
	var candidate []byte
	if candidate, err = buf.ReadUntil('"'); log.Fail(err) {
		return
	}
	// log.D.F("label: '%s' %v", string(candidate), List)
	var differs bool
matched:
	for i := range labels.List {
		differs = false
		if len(candidate) == len(labels.List[i]) {
			for j := range candidate {
				if candidate[j] != labels.List[i][j] {
					differs = true
					break
				}
			}
			if !differs {
				// there can only be one!
				match = string(labels.List[i])
				break matched
			}
		}
	}
	// if there was no match we still have zero.
	if match == "" {
		// no match
		err = fmt.Errorf("label '%s' not recognised as envelope label",
			string(candidate))
		return
	}
	log.T.F("received %s envelope", match)
	return
}

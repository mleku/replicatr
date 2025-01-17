package authenvelope

import (
	"net/url"
	"strings"
	"time"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kind"
)

// Validate checks whether event is a valid NIP-42 event for given challenge and
// relayURL. The result of the validation is encoded in the ok bool.
func Validate(evt *event.T, challenge string,
	relayURL string) (pubkey string, ok bool) {

	if evt.Kind != kind.ClientAuthentication {
		return "", false
	}
	if evt.Tags.GetFirst([]string{"challenge", challenge}) == nil {
		return "", false
	}
	var expected, found *url.URL
	var err error
	expected, err = parseURL(relayURL)
	if err != nil {
		return "", false
	}
	found, err = parseURL(evt.Tags.GetFirst([]string{"relay", ""}).Value())
	if err != nil {
		return "", false
	}
	if expected.Scheme != found.Scheme ||
		expected.Host != found.Host ||
		expected.Path != found.Path {
		return "", false
	}
	now := time.Now()
	if evt.CreatedAt.Time().After(now.Add(10*time.Minute)) ||
		evt.CreatedAt.Time().Before(now.Add(-10*time.Minute)) {

		return "", false
	}
	if ok, err = evt.CheckSignature(); !ok || log.Fail(err) {
		return "", false
	}
	return evt.PubKey, true
}

// helper function for Validate.
func parseURL(input string) (*url.URL, error) {
	return url.Parse(
		strings.ToLower(
			strings.TrimSuffix(input, "/"),
		),
	)
}

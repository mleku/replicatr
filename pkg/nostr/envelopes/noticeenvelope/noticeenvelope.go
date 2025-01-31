package noticeenvelope

import (
	"fmt"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/envelopes/labels"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/interfaces/enveloper"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/wire/array"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/wire/text"
	"mleku.online/git/slog"
)

var log = slog.GetStd()

// T is a relay message intended to be shown to users in a nostr
// client interface.
type T struct {
	Text string
}

var _ enveloper.I = (*T)(nil)

func (E *T) UnmarshalJSON(bytes []byte) error {
	// TODO implement me
	panic("implement me")
}

func NewNoticeEnvelope(text string) (E *T) {
	E = &T{Text: text}
	return
}

// Label returns the label enum/type of the envelope. The relevant bytes could
// be retrieved using nip1.List[T]
func (E *T) Label() string { return labels.NOTICE }

func (E *T) ToArray() array.T { return array.T{labels.NOTICE, E.Text} }

func (E *T) String() (s string) { return E.ToArray().String() }

func (E *T) Bytes() (s []byte) { return E.ToArray().Bytes() }

func (E *T) MarshalJSON() ([]byte, error) { return E.Bytes(), nil }

// Unmarshal the envelope.
func (E *T) Unmarshal(buf *text.Buffer) (err error) {
	if E == nil {
		return fmt.Errorf("cannot unmarshal to nil pointer")
	}
	// Next, find the comma after the label (note we aren't checking that only
	// whitespace intervenes because laziness, usually this is the very next
	// character).
	if err = buf.ScanUntil(','); err != nil {
		return
	}
	// Next character we find will be open quotes for the notice text.
	if err = buf.ScanThrough('"'); err != nil {
		return
	}
	var noticeText []byte
	// read the string
	if noticeText, err = buf.ReadUntil('"'); log.Fail(err) {
		return fmt.Errorf("unterminated quotes in JSON, probably truncated read")
	}
	E.Text = string(text.UnescapeByteString(noticeText))
	// log.D.F("'%s'", E.Text)
	return
}

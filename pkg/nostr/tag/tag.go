package tag

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/normalize"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/wire/text"
)

// The tag position meanings so they are clear when reading.
const (
	Key = iota
	Value
	Relay
)

// T marker strings for e (reference) tags.
const (
	MarkerReply   = "reply"
	MarkerRoot    = "root"
	MarkerMention = "mention"
)

// T is a list of strings with a literal ordering.
//
// Not a set, there can be repeating elements.
type T []string

// StartsWith checks a tag has the same initial set of elements.
//
// The last element is treated specially in that it is considered to match if
// the candidate has the same initial substring as its corresponding element.
func (t T) StartsWith(prefix []string) bool {
	prefixLen := len(prefix)

	if prefixLen > len(t) {
		return false
	}
	// check initial elements for equality
	for i := 0; i < prefixLen-1; i++ {
		if prefix[i] != t[i] {
			return false
		}
	}
	// check last element just for a prefix
	return strings.HasPrefix(t[prefixLen-1], prefix[prefixLen-1])
}

// Key returns the first element of the tags.
func (t T) Key() string {
	if len(t) > Key {
		return t[Key]
	}
	return ""
}

// Value returns the second element of the tag.
func (t T) Value() string {
	if len(t) > Value {
		return t[Value]
	}
	return ""
}

// Relay returns the third element of the tag.
func (t T) Relay() string {
	if (t.Key() == "e" || t.Key() == "p") && len(t) > Relay {
		return normalize.URL(t[Relay])
	}
	return ""
}

// MarshalTo T. Used for Serialization so string escaping should be as in
// RFC8259.
func (t T) MarshalTo(dst []byte) []byte {
	dst = append(dst, '[')
	for i, s := range t {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, text.EscapeJSONStringAndWrap(s)...)
	}
	dst = append(dst, ']')
	return dst
}

func (t T) String() string {
	buf := new(bytes.Buffer)
	buf.WriteByte('[')
	last := len(t) - 1
	for i := range t {
		buf.WriteByte('"')
		_, _ = fmt.Fprint(buf, t[i])
		buf.WriteByte('"')
		if i < last {
			buf.WriteByte(',')
		}
	}
	buf.WriteByte(']')
	return buf.String()
}

// Clone makes a new tag.T with the same members.
func (t T) Clone() (c T) {
	c = make(T, len(t))
	for i := range t {
		c[i] = t[i]
	}
	return
}

// Contains returns true if the provided element is found in the tag slice.
func (t T) Contains(s string) bool {
	for i := range t {
		if t[i] == s {
			return true
		}
	}
	return false
}

// Equals checks that the provided tag list matches.
func (t T) Equals(t1 T) bool {
	if len(t) != len(t1) {
		return false
	}
	for i := range t {
		if t[i] != t1[i] {
			return false
		}
	}
	return true
}

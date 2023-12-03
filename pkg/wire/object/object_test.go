package object

import (
	"encoding/json"
	"mleku.online/git/replicatr/pkg/nostr/kind"
	"mleku.online/git/replicatr/pkg/nostr/tags"
	"mleku.online/git/replicatr/pkg/nostr/timestamp"
	"mleku.online/git/replicatr/pkg/wire/array"
	"testing"
	"time"
)

var literal = T{
	{"1", "aoeu"},
	{"3", time.Now()},
	{"sorta normal", 0.333},
}

// Event is redefined here to avoid a dependency.
type Event struct {
	ID        string
	PubKey    string
	CreatedAt timestamp.T
	Kind      kind.T
	Tags      tags.T
	Content   string
	Sig       string
}

var structLiteral = array.T{"EVENT", Event{
	ID:        "this is id",
	PubKey:    "this is pubkey",
	CreatedAt: timestamp.Now(),
	Kind:      kind.MemoryHole,
	Tags: tags.T{
		{"e", "something", "something/else"},
		{"e", "something", "something/else"},
	},
	Content: "this is content",
	Sig:     "this is sig",
}}

func EventToObject(ev array.T) (t array.T) {
	return array.T{
		ev[0],
		T{
			{"id", ev[1].(Event).ID},
			{"pubkey", ev[1].(Event).PubKey},
			{"created_at", ev[1].(Event).CreatedAt},
			{"kind", ev[1].(Event).Kind},
			{"tags", ev[1].(Event).Tags},
			{"content", ev[1].(Event).Content},
			{"sig", ev[1].(Event).Sig},
		},
	}
}

func TestObject(t *testing.T) {
	var b []byte
	var e error
	b, e = json.Marshal(literal)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(string(b))
	t.Log(literal)
}

func TestEventToObject(t *testing.T) {

	// This demonstrates how the array.T and object.T correctly returning
	// canonical JSON.
	//
	// To implement this any type one needs to create a strictly ordered JSON
	// version of the data must create the function like EventToObject above
	// which in this case is quite artificial, as a real version of this would
	// be able to string together multiple events in the envelope as per NIP-1
	//
	// Note in the output printed to the logger in this test, that json tags do
	// not have to be specified but instead the mapping is created via the
	// object.T conversion function, as those were omitted from the above
	// reproduction of the Event struct, they are imputed to the same string as
	// the variable name as the encoding/json library does, due to its use of
	// reflect.

	obj := EventToObject(structLiteral)
	var b []byte
	var e error
	b, e = json.Marshal(structLiteral)
	if e != nil {
		t.Fatal(e)
	}
	var ifc interface{}
	e = json.Unmarshal(b, &ifc)
	if e != nil {
		t.Fatal(e)
	}
	b, e = json.Marshal(ifc)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("wrong", string(b))
	t.Log("right", obj)
}
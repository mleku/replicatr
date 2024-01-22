package badger

import (
	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/eventstore"

	"github.com/Hubmakerlabs/replicatr/pkg/hex"
	nostr_binary "github.com/Hubmakerlabs/replicatr/pkg/nostr/binary"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/dgraph-io/badger/v4"
)

func (b *BadgerBackend) SaveEvent(c context.T, evt *event.T) (e error) {
	return b.Update(func(txn *badger.Txn) (e error) {
		// query event by id to ensure we don't save duplicates
		id, _ := hex.Dec(evt.ID.String())
		prefix := make([]byte, 1+8)
		prefix[0] = indexIdPrefix
		copy(prefix[1:], id)
		it := txn.NewIterator(badger.IteratorOptions{})
		defer it.Close()
		it.Seek(prefix)
		if it.ValidForPrefix(prefix) {
			// event exists
			return eventstore.ErrDupEvent
		}

		// encode to binary
		var bin []byte
		if bin, e = nostr_binary.Marshal(evt); log.Fail(e) {
			return e
		}

		idx := b.Serial()
		// raw event store
		if e = txn.Set(idx, bin); e != nil {
			return e
		}

		for _, k := range getIndexKeysForEvent(evt, idx[1:]) {
			if e = txn.Set(k, nil); e != nil {
				return e
			}
		}

		return nil
	})
}

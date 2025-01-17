package badger

import (
	"github.com/Hubmakerlabs/replicatr/pkg/context"

	"github.com/Hubmakerlabs/replicatr/pkg/hex"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/dgraph-io/badger/v4"
)

var serialDelete uint32 = 0

func (b *BadgerBackend) DeleteEvent(c context.T, evt *event.T) (err error) {
	deletionHappened := false

	err = b.Update(func(txn *badger.Txn) (err error) {
		idx := make([]byte, 1, 5)
		idx[0] = rawEventStorePrefix

		// query event by id to get its idx
		idPrefix8, _ := hex.Dec(evt.ID[0 : 8*2].String())
		prefix := make([]byte, 1+8)
		prefix[0] = indexIdPrefix
		copy(prefix[1:], idPrefix8)
		opts := badger.IteratorOptions{
			PrefetchValues: false,
		}
		it := txn.NewIterator(opts)
		it.Seek(prefix)
		if it.ValidForPrefix(prefix) {
			idx = append(idx, it.Item().Key()[1+8:]...)
		}
		it.Close()

		// if no idx was found, end here, this event doesn't exist
		if len(idx) == 1 {
			return nil
		}

		// set this so we'll run the GC later
		deletionHappened = true

		// calculate all index keys we have for this event and delete them
		for _, k := range getIndexKeysForEvent(evt, idx[1:]) {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}

		// delete the raw event
		return txn.Delete(idx)
	})
	if err != nil {
		return err
	}

	// after deleting, run garbage collector (sometimes)
	if deletionHappened {
		serialDelete = (serialDelete + 1) % 256
		if serialDelete == 0 {
			if err := b.RunValueLogGC(0.8); err != nil && err != badger.ErrNoRewrite {
				log.D.Ln("badger gc error:" + err.Error())
			}
		}
	}

	return nil
}

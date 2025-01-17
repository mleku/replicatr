package badger

import (
	"encoding/binary"
	"errors"

	"github.com/Hubmakerlabs/replicatr/pkg/context"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filter"
	nostr_binary "github.com/Hubmakerlabs/replicatr/pkg/nostr/nostrbinary"
	"github.com/dgraph-io/badger/v4"
)

func (b *BadgerBackend) CountEvents(c context.T, f *filter.T) (int64, error) {
	var count int64 = 0

	queries, extraFilter, since, err := prepareQueries(f)
	if err != nil {
		return 0, err
	}

	err = b.View(func(txn *badger.Txn) (err error) {
		// iterate only through keys and in reverse order
		opts := badger.IteratorOptions{
			Reverse: true,
		}

		// actually iterate
		for _, q := range queries {
			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek(q.startingPoint); it.ValidForPrefix(q.prefix); it.Next() {
				item := it.Item()
				key := item.Key()

				idxOffset := len(key) - 4 // this is where the idx actually starts

				// "id" indexes don't contain a timestamp
				if !q.skipTimestamp {
					createdAt := binary.BigEndian.Uint32(key[idxOffset-4 : idxOffset])
					if createdAt < since {
						break
					}
				}

				idx := make([]byte, 5)
				idx[0] = rawEventStorePrefix
				copy(idx[1:], key[idxOffset:])

				if extraFilter == nil {
					count++
				} else {
					// fetch actual event
					item, err = txn.Get(idx)
					if err != nil {
						if errors.Is(err, badger.ErrDiscardedTxn) {
							return
						}
						log.D.F("badger: count (%v) failed to get %d from raw "+
							"event store: %s", q, idx)
						return
					}

					err = item.Value(func(val []byte) (err error) {
						var evt *event.T
						if evt, err = nostr_binary.Unmarshal(val); err != nil {
							return err
						}

						// check if this matches the other filters that were not part of the index
						if extraFilter == nil || extraFilter.Matches(evt) {
							count++
						}

						return nil
					})
					if log.Fail(err) {
						log.D.F("badger: count value read error: %s", err)
					}
				}
			}
		}

		return nil
	})

	return count, err
}

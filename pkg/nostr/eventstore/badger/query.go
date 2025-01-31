package badger

import (
	"container/heap"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Hubmakerlabs/replicatr/pkg/context"
	"github.com/Hubmakerlabs/replicatr/pkg/hex"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/filter"
	nostr_binary "github.com/Hubmakerlabs/replicatr/pkg/nostr/nostrbinary"
	"github.com/dgraph-io/badger/v4"
)

type query struct {
	i             int
	prefix        []byte
	startingPoint []byte
	results       chan *event.T
	skipTimestamp bool
}

type queryEvent struct {
	*event.T
	query int
}

func (b *BadgerBackend) QueryEvents(c context.T, f *filter.T) (chan *event.T, error) {
	ch := make(chan *event.T)

	queries, extraFilter, since, err := prepareQueries(f)
	if err != nil {
		return nil, err
	}

	go func() {
		err := b.View(func(txn *badger.Txn) (err error) {
			// iterate only through keys and in reverse order
			opts := badger.IteratorOptions{
				Reverse: true,
			}

			// actually iterate
			iteratorClosers := make([]func(), len(queries))
			for i, q := range queries {
				go func(i int, q query) {
					it := txn.NewIterator(opts)
					iteratorClosers[i] = it.Close

					defer close(q.results)

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

						// fetch actual event
						item, err = txn.Get(idx)
						if err != nil {
							if errors.Is(err, badger.ErrDiscardedTxn) {
								return
							}
							b.D.F("badger: failed to get %x based on prefix %x, index key %x from raw event store: %s",
								idx, q.prefix, key, err)
							return
						}
						b.Fail(item.Value(func(val []byte) (err error) {
							var evt *event.T
							if evt, err = nostr_binary.Unmarshal(val); err != nil {
								b.D.F("badger: value read error (id %x): %s", val[0:32], err)
								return err
							}

							// check if this matches the other filters that were not part of the index
							if extraFilter == nil || extraFilter.Matches(evt) {
								q.results <- evt
							}

							return nil
						}))
					}
				}(i, q)
			}

			// max number of events we'll return
			limit := b.MaxLimit
			if f.Limit > 0 && f.Limit < limit {
				limit = f.Limit
			}

			// receive results and ensure we only return the most recent ones always
			emittedEvents := 0

			// first pass
			emitQueue := make(priorityQueue, 0, len(queries)+limit)
			for _, q := range queries {
				evt, ok := <-q.results
				if ok {
					emitQueue = append(emitQueue, &queryEvent{T: evt, query: q.i})
				}
			}

			// now it's a good time to schedule this
			defer func() {
				close(ch)
				for _, itclose := range iteratorClosers {
					itclose()
				}
			}()

			// queue may be empty here if we have literally nothing
			if len(emitQueue) == 0 {
				return nil
			}

			heap.Init(&emitQueue)

			// iterate until we've emitted all events required
			for {
				// emit latest event in queue
				latest := emitQueue[0]
				ch <- latest.T

				// stop when reaching limit
				emittedEvents++
				if emittedEvents == limit {
					break
				}

				// fetch a new one from query results and replace the previous one with it
				if evt, ok := <-queries[latest.query].results; ok {
					emitQueue[0].T = evt
					heap.Fix(&emitQueue, 0)
				} else {
					// if this query has no more events we just remove this and proceed normally
					heap.Remove(&emitQueue, 0)

					// check if the list is empty and end
					if len(emitQueue) == 0 {
						break
					}
				}
			}

			return nil
		})
		if err != nil {
			b.D.F("badger: query txn error: %s", err)
		}
	}()

	return ch, nil
}

type priorityQueue []*queryEvent

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].CreatedAt > pq[j].CreatedAt
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x any) {
	item := x.(*queryEvent)
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*pq = old[0 : n-1]
	return item
}

func prepareQueries(f *filter.T) (
	queries []query,
	extraFilter *filter.T,
	since uint32,
	err error,
) {
	var index byte

	if len(f.IDs) > 0 {
		index = indexIdPrefix
		queries = make([]query, len(f.IDs))
		for i, idHex := range f.IDs {
			prefix := make([]byte, 1+8)
			prefix[0] = index
			if len(idHex) != 64 {
				return nil, nil, 0, fmt.Errorf("invalid id '%s'", idHex)
			}
			idPrefix8, _ := hex.Dec(idHex[0 : 8*2])
			copy(prefix[1:], idPrefix8)
			queries[i] = query{i: i, prefix: prefix, skipTimestamp: true}
		}
	} else if len(f.Authors) > 0 {
		if len(f.Kinds) == 0 {
			index = indexPubkeyPrefix
			queries = make([]query, len(f.Authors))
			for i, pubkeyHex := range f.Authors {
				if len(pubkeyHex) != 64 {
					return nil, nil, 0, fmt.Errorf("invalid pubkey '%s'", pubkeyHex)
				}
				pubkeyPrefix8, _ := hex.Dec(pubkeyHex[0 : 8*2])
				prefix := make([]byte, 1+8)
				prefix[0] = index
				copy(prefix[1:], pubkeyPrefix8)
				queries[i] = query{i: i, prefix: prefix}
			}
		} else {
			index = indexPubkeyKindPrefix
			queries = make([]query, len(f.Authors)*len(f.Kinds))
			i := 0
			for _, pubkeyHex := range f.Authors {
				for _, kind := range f.Kinds {
					if len(pubkeyHex) != 64 {
						return nil, nil, 0, fmt.Errorf("invalid pubkey '%s'", pubkeyHex)
					}
					pubkeyPrefix8, _ := hex.Dec(pubkeyHex[0 : 8*2])
					prefix := make([]byte, 1+8+2)
					prefix[0] = index
					copy(prefix[1:], pubkeyPrefix8)
					binary.BigEndian.PutUint16(prefix[1+8:], uint16(kind))
					queries[i] = query{i: i, prefix: prefix}
					i++
				}
			}
		}
		extraFilter = &filter.T{Tags: f.Tags}
	} else if len(f.Tags) > 0 {
		// determine the size of the queries array by inspecting all tags sizes
		size := 0
		for _, values := range f.Tags {
			size += len(values)
		}
		if size == 0 {
			return nil, nil, 0, fmt.Errorf("empty tag filters")
		}

		queries = make([]query, size)

		extraFilter = &filter.T{Kinds: f.Kinds}
		i := 0
		for _, values := range f.Tags {
			for _, value := range values {
				// get key prefix (with full length) and offset where to write the last parts
				k, offset := getTagIndexPrefix(value)
				// remove the last parts part to get just the prefix we want here
				prefix := k[0:offset]

				queries[i] = query{i: i, prefix: prefix}
				i++
			}
		}
	} else if len(f.Kinds) > 0 {
		index = indexKindPrefix
		queries = make([]query, len(f.Kinds))
		for i, kind := range f.Kinds {
			prefix := make([]byte, 1+2)
			prefix[0] = index
			binary.BigEndian.PutUint16(prefix[1:], uint16(kind))
			queries[i] = query{i: i, prefix: prefix}
		}
	} else {
		index = indexCreatedAtPrefix
		queries = make([]query, 1)
		prefix := make([]byte, 1)
		prefix[0] = index
		queries[0] = query{i: 0, prefix: prefix}
		extraFilter = nil
	}

	var until uint32 = 4294967295
	if f.Until != nil {
		if fu := uint32(*f.Until); fu < until {
			until = fu + 1
		}
	}
	for i, q := range queries {
		queries[i].startingPoint = binary.BigEndian.AppendUint32(q.prefix, uint32(until))
		queries[i].results = make(chan *event.T, 12)
	}

	// this is where we'll end the iteration
	if f.Since != nil {
		if fs := uint32(*f.Since); fs > since {
			since = fs
		}
	}

	return queries, extraFilter, since, nil
}

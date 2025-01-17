package nostrbinary

import (
	"bytes"
	"encoding/gob"
	"os"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"mleku.online/git/slog"
)

var log = slog.New(os.Stderr, "nostrbinary")

func Unmarshal(data []byte) (evt *event.T, err error) {

	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	evt = &event.T{}
	if err = dec.Decode(evt); log.Fail(err) {
		return
	}
	return

	// defer func() {
	// 	if r := recover(); r != nil {
	// 		e = fmt.Errorf("failed to decode binary: %v", r)
	// 	}
	// }()
	//
	// evt.ID = eventid.T(hex.EncodeToString(data[0:32]))
	// evt.PubKey = hex.EncodeToString(data[32:64])
	// evt.Sig = hex.EncodeToString(data[64:128])
	// evt.CreatedAt = timestamp.T(binary.BigEndian.Uint32(data[128:132]))
	// evt.Kind = kind.T(binary.BigEndian.Uint16(data[132:134]))
	// contentLength := int(binary.BigEndian.Uint16(data[134:136]))
	// evt.Content = string(data[136 : 136+contentLength])
	//
	// curr := 136 + contentLength
	//
	// nTags := binary.BigEndian.Uint16(data[curr : curr+2])
	// curr++
	// evt.Tags = make(tags.T, nTags)
	//
	// for t := range evt.Tags {
	// 	curr++
	// 	nItems := int(data[curr])
	// 	tag := make(tag.T, nItems)
	// 	for i := range tag {
	// 		curr = curr + 1
	// 		itemSize := int(binary.BigEndian.Uint16(data[curr : curr+2]))
	// 		itemStart := curr + 2
	// 		itemEnd := itemStart + itemSize
	// 		item := string(data[itemStart:itemEnd])
	// 		tag[i] = item
	// 		curr = itemEnd
	// 	}
	// 	evt.Tags[t] = tag
	// }
	//
	// return e
}

func Marshal(evt *event.T) (b []byte, err error) {

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(evt); log.Fail(err) {
		return
	}
	b = buf.Bytes()
	return

	// content := []byte(evt.Content)
	// buf := make([]byte, 32+32+64+4+2+2+len(content)+65536 /* blergh */)
	//
	// hex.Decode(buf[0:32], []byte(evt.ID))
	// hex.Decode(buf[32:64], []byte(evt.PubKey))
	// hex.Decode(buf[64:128], []byte(evt.Sig))
	//
	// if evt.CreatedAt > MaxCreatedAt {
	// 	return nil, fmt.Errorf("created_at is too big: %d, max is %d",
	// 		evt.CreatedAt, MaxCreatedAt)
	// }
	// binary.BigEndian.PutUint32(buf[128:132], uint32(evt.CreatedAt))
	//
	// if evt.Kind > MaxKind {
	// 	return nil, fmt.Errorf("kind is too big: %d, max is %d", evt.Kind,
	// 		MaxKind)
	// }
	// binary.BigEndian.PutUint16(buf[132:134], uint16(evt.Kind))
	//
	// if contentLength := len(content); contentLength > MaxContentSize {
	// 	return nil, fmt.Errorf("content is too large: %d, max is %d",
	// 		contentLength, MaxContentSize)
	// } else {
	// 	binary.BigEndian.PutUint16(buf[134:136], uint16(contentLength))
	// }
	// copy(buf[136:], content)
	//
	// curr := 136 + len(content)
	//
	// if tagCount := len(evt.Tags); tagCount > MaxTagCount {
	// 	return nil, fmt.Errorf("can't encode too many tags: %d, max is %d",
	// 		tagCount, MaxTagCount)
	// } else {
	// 	binary.BigEndian.PutUint16(buf[curr:curr+2], uint16(tagCount))
	// }
	// curr++
	//
	// for _, tag := range evt.Tags {
	// 	curr++
	// 	if itemCount := len(tag); itemCount > MaxTagItemCount {
	// 		return nil, fmt.Errorf("can't encode a tag with so many items: %d, max is %d",
	// 			itemCount, MaxTagItemCount)
	// 	} else {
	// 		buf[curr] = uint8(itemCount)
	// 	}
	// 	for _, item := range tag {
	// 		curr++
	// 		itemb := []byte(item)
	// 		itemSize := len(itemb)
	// 		if itemSize > MaxTagItemSize {
	// 			return nil, fmt.Errorf("tag item is too large: %d, max is %d",
	// 				itemSize, MaxTagItemSize)
	// 		}
	// 		binary.BigEndian.PutUint16(buf[curr:curr+2], uint16(itemSize))
	// 		itemEnd := curr + 2 + itemSize
	// 		copy(buf[curr+2:itemEnd], itemb)
	// 		curr = itemEnd
	// 	}
	// }
	// buf = buf[0 : curr+1]
	// return buf, nil
}

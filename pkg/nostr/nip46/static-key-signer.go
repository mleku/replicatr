package nip46

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Hubmakerlabs/replicatr/pkg/nostr/event"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/keys"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kind"
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/nip4"
	"golang.org/x/exp/slices"
)

var _ Signer = (*StaticKeySigner)(nil)

type StaticKeySigner struct {
	secretKey string

	sessionKeys []string
	sessions    []Session

	sync.Mutex

	RelaysToAdvertise map[string]RelayReadWrite
}

func NewStaticKeySigner(secretKey string) StaticKeySigner {
	return StaticKeySigner{
		secretKey:         secretKey,
		RelaysToAdvertise: make(map[string]RelayReadWrite),
	}
}

func (p *StaticKeySigner) GetSession(clientPubkey string) (Session, bool) {
	idx, exists := slices.BinarySearch(p.sessionKeys, clientPubkey)
	if exists {
		return p.sessions[idx], true
	}
	return Session{}, false
}

func (p *StaticKeySigner) getOrCreateSession(clientPubkey string) (Session, error) {
	p.Lock()
	defer p.Unlock()

	idx, exists := slices.BinarySearch(p.sessionKeys, clientPubkey)
	if exists {
		return p.sessions[idx], nil
	}

	shared, err := nip4.ComputeSharedSecret(clientPubkey, p.secretKey)
	if err != nil {
		return Session{}, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	session := Session{
		SharedKey: shared,
	}

	// add to pool
	p.sessionKeys = append(p.sessionKeys, "") // bogus append just to increase the capacity
	p.sessions = append(p.sessions, Session{})
	copy(p.sessionKeys[idx+1:], p.sessionKeys[idx:])
	copy(p.sessions[idx+1:], p.sessions[idx:])
	p.sessionKeys[idx] = clientPubkey
	p.sessions[idx] = session

	return session, nil
}

func (p *StaticKeySigner) HandleRequest(ev *event.T) (
	req Request,
	resp Response,
	eventResponse *event.T,
	harmless bool,
	err error,
) {
	if ev.Kind != kind.NostrConnect {
		return req, resp, eventResponse, false,
			fmt.Errorf("event kind is %d, but we expected %d",
				ev.Kind, kind.NostrConnect)
	}

	session, err := p.getOrCreateSession(ev.PubKey)
	if err != nil {
		return req, resp, eventResponse, false, err
	}

	req, err = session.ParseRequest(ev)
	if err != nil {
		return req, resp, eventResponse, false,
			fmt.Errorf("error parsing request: %w", err)
	}

	var result string
	var resultErr error

	switch req.Method {
	case "connect":
		result = "ack"
		harmless = true
	case "get_public_key":
		pubkey, err := keys.GetPublicKey(p.secretKey)
		if err != nil {
			resultErr = fmt.Errorf("failed to derive public key: %w", err)
			break
		} else {
			result = pubkey
			harmless = true
		}
	case "sign_event":
		if len(req.Params) != 1 {
			resultErr = fmt.Errorf("wrong number of arguments to 'sign_event'")
			break
		}
		evt := &event.T{}
		err = json.Unmarshal([]byte(req.Params[0]), evt)
		if err != nil {
			resultErr = fmt.Errorf("failed to decode event/2: %w", err)
			break
		}
		err = evt.Sign(p.secretKey)
		if err != nil {
			resultErr = fmt.Errorf("failed to sign event: %w", err)
			break
		}
		jrevt, _ := json.Marshal(evt)
		result = string(jrevt)
	case "get_relays":
		jrelays, _ := json.Marshal(p.RelaysToAdvertise)
		result = string(jrelays)
		harmless = true
	case "nip04_encrypt":
		if len(req.Params) != 2 {
			resultErr = fmt.Errorf("wrong number of arguments to 'nip04_encrypt'")
			break
		}
		thirdPartyPubkey := req.Params[0]
		if !keys.IsValid32ByteHex(thirdPartyPubkey) {
			resultErr = fmt.Errorf("first argument to 'nip04_encrypt' is not a pubkey string")
			break
		}
		plaintext := req.Params[1]
		sharedSecret, err := nip4.ComputeSharedSecret(thirdPartyPubkey, p.secretKey)
		if err != nil {
			resultErr = fmt.Errorf("failed to compute shared secret: %w", err)
			break
		}
		ciphertext, err := nip4.Encrypt(plaintext, sharedSecret)
		if err != nil {
			resultErr = fmt.Errorf("failed to encrypt: %w", err)
			break
		}
		result = ciphertext
	case "nip04_decrypt":
		if len(req.Params) != 2 {
			resultErr = fmt.Errorf("wrong number of arguments to 'nip04_decrypt'")
			break
		}
		thirdPartyPubkey := req.Params[0]
		if !keys.IsValid32ByteHex(thirdPartyPubkey) {
			resultErr = fmt.Errorf("first argument to 'nip04_decrypt' is not a pubkey string")
			break
		}
		ciphertext := req.Params[1]
		sharedSecret, err := nip4.ComputeSharedSecret(thirdPartyPubkey, p.secretKey)
		if err != nil {
			resultErr = fmt.Errorf("failed to compute shared secret: %w", err)
			break
		}
		plaintext, err := nip4.Decrypt(ciphertext, sharedSecret)
		if err != nil {
			resultErr = fmt.Errorf("failed to encrypt: %w", err)
			break
		}
		result = string(plaintext)
	default:
		return req, resp, eventResponse, false,
			fmt.Errorf("unknown method '%s'", req.Method)
	}

	resp, eventResponse, err = session.MakeResponse(req.ID, ev.PubKey, result, resultErr)
	if err != nil {
		return req, resp, eventResponse, harmless, err
	}

	err = eventResponse.Sign(p.secretKey)
	if err != nil {
		return req, resp, eventResponse, harmless, err
	}

	return req, resp, eventResponse, harmless, err
}

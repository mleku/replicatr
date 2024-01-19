package nip11

import (
	"github.com/Hubmakerlabs/replicatr/pkg/nostr/kinds"
	"golang.org/x/exp/slices"
)

type Limits struct {
	MaxMessageLength int  `json:"max_message_length,omitempty"`
	MaxSubscriptions int  `json:"max_subscriptions,omitempty"`
	MaxFilters       int  `json:"max_filters,omitempty"`
	MaxLimit         int  `json:"max_limit,omitempty"`
	MaxSubidLength   int  `json:"max_subid_length,omitempty"`
	MaxEventTags     int  `json:"max_event_tags,omitempty"`
	MaxContentLength int  `json:"max_content_length,omitempty"`
	MinPowDifficulty int  `json:"min_pow_difficulty,omitempty"`
	AuthRequired     bool `json:"auth_required"`
	PaymentRequired  bool `json:"payment_required"`
	RestrictedWrites bool `json:"restricted_writes"`
}
type Payment struct {
	Amount int    `json:"amount"`
	Unit   string `json:"unit"`
}

type Sub struct {
	Payment
	Period int `json:"period"`
}

type Pub struct {
	Kinds kinds.T `json:"kinds"`
	Payment
}

type Fees struct {
	Admission    []Payment `json:"admission,omitempty"`
	Subscription []Sub     `json:"subscription,omitempty"`
	Publication  []Pub     `json:"publication,omitempty"`
}

type Info struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	PubKey         string   `json:"pubkey"`
	Contact        string   `json:"contact"`
	SupportedNIPs  []int    `json:"supported_nips"`
	Software       string   `json:"software"`
	Version        string   `json:"version"`
	Limitation     *Limits  `json:"limitation,omitempty"`
	RelayCountries []string `json:"relay_countries,omitempty"`
	LanguageTags   []string `json:"language_tags,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	PostingPolicy  string   `json:"posting_policy,omitempty"`
	PaymentsURL    string   `json:"payments_url,omitempty"`
	Fees           *Fees    `json:"fees,omitempty"`
	Icon           string   `json:"icon"`
}

func (info *Info) AddSupportedNIP(number int) {
	idx, exists := slices.BinarySearch(info.SupportedNIPs, number)
	if exists {
		return
	}

	info.SupportedNIPs = append(info.SupportedNIPs, -1)
	copy(info.SupportedNIPs[idx+1:], info.SupportedNIPs[idx:])
	info.SupportedNIPs[idx] = number
}
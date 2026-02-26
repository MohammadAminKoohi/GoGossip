package message

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const CurrentVersion = 1

type MessageType string

const (
	MessageTypeHello    MessageType = "HELLO"
	MessageTypeGetPeers MessageType = "GET_PEERS"
	MessageTypePeers    MessageType = "PEERS_LIST"
	MessageTypeGossip   MessageType = "GOSSIP"
	MessageTypePing     MessageType = "PING"
	MessageTypePong     MessageType = "PONG"
	MessageTypeIHave    MessageType = "IHAVE"
	MessageTypeIWant    MessageType = "IWANT"
)

type Envelope struct {
	Version    int             `json:"version"`
	MsgID      string          `json:"msg_id"`
	MsgType    MessageType     `json:"msg_type"`
	SenderID   string          `json:"sender_id"`
	SenderAddr string          `json:"sender_addr"`
	Timestamp  int64           `json:"timestamp_ms"`
	TTL        int             `json:"ttl"`
	Payload    json.RawMessage `json:"payload"`
}

type PeerInfo struct {
	NodeID string `json:"node_id"`
	Addr   string `json:"addr"`
}

type PoWProof struct {
	HashAlg     string `json:"hash_alg"`
	DifficultyK int    `json:"difficulty_k"`
	Nonce       uint64 `json:"nonce"`
	DigestHex   string `json:"digest_hex"`
}

type HelloPayload struct {
	Capabilities []string  `json:"capabilities"`
	PoW          *PoWProof `json:"pow,omitempty"`
}

type GetPeersPayload struct {
	MaxPeers int `json:"max_peers"`
}

type PeersListPayload struct {
	Peers []PeerInfo `json:"peers"`
}

type GossipPayload struct {
	Topic              string          `json:"topic"`
	Data               json.RawMessage `json:"data"`
	OriginID           string          `json:"origin_id"`
	OriginTimestampMs  int64           `json:"origin_timestamp_ms"`
}

type PingPayload struct {
	PingID string `json:"ping_id"`
	Seq    int64  `json:"seq"`
}

type PongPayload struct {
	PingID string `json:"ping_id"`
	Seq    int64  `json:"seq"`
}

// IHavePayload is sent periodically to tell neighbors which message IDs we have.
type IHavePayload struct {
	IDs    []string `json:"ids"`
	MaxIDs int      `json:"max_ids"`
}

// IWantPayload requests full GOSSIP messages for the given IDs.
type IWantPayload struct {
	IDs []string `json:"ids"`
}

func NewEnvelope(msgType MessageType, senderID, senderAddr string, ttl int, payload any) (Envelope, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal payload: %w", err)
	}

	env := Envelope{
		Version:    CurrentVersion,
		MsgID:      newMsgID(),
		MsgType:    msgType,
		SenderID:   senderID,
		SenderAddr: senderAddr,
		Timestamp:  time.Now().UnixMilli(),
		TTL:        ttl,
		Payload:    payloadBytes,
	}

	if err := env.Validate(); err != nil {
		return Envelope{}, err
	}

	return env, nil
}

func (e Envelope) Encode() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(e)
}

func DecodeEnvelope(data []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("unmarshal envelope: %w", err)
	}

	if err := env.Validate(); err != nil {
		return Envelope{}, err
	}

	return env, nil
}

func DecodePayload[T any](e Envelope) (T, error) {
	var payload T
	if len(e.Payload) == 0 {
		return payload, errors.New("payload is empty")
	}

	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return payload, fmt.Errorf("unmarshal payload: %w", err)
	}

	return payload, nil
}

func (e Envelope) Validate() error {
	if e.Version != CurrentVersion {
		return fmt.Errorf("unsupported version: %d", e.Version)
	}

	if !e.MsgType.IsValid() {
		return fmt.Errorf("invalid msg_type: %q", e.MsgType)
	}

	if e.MsgID == "" {
		return errors.New("msg_id is required")
	}

	if e.SenderID == "" {
		return errors.New("sender_id is required")
	}

	if e.SenderAddr == "" {
		return errors.New("sender_addr is required")
	}

	if e.Timestamp <= 0 {
		return errors.New("timestamp_ms must be > 0")
	}

	if e.TTL < 0 {
		return errors.New("ttl must be >= 0")
	}

	if len(e.Payload) == 0 {
		return errors.New("payload is required")
	}

	return nil
}

func (t MessageType) IsValid() bool {
	switch t {
	case MessageTypeHello, MessageTypeGetPeers, MessageTypePeers, MessageTypeGossip, MessageTypePing, MessageTypePong, MessageTypeIHave, MessageTypeIWant:
		return true
	default:
		return false
	}
}

func newMsgID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}

	return hex.EncodeToString(raw)
}

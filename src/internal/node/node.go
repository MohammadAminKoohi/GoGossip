package node

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/google/uuid"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/network"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/peer"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/seen"
)

type Config struct {
	Port         int
	Bootstrap    string
	Fanout       int
	TTL          int
	PeerLimit    int
	PingInterval int
	PeerTimeout  int
	Seed         int64
}

type Node struct {
	cfg      Config
	selfAddr string
	uuid     uuid.UUID

	peers *peer.Store
	seen  *seen.Set
}

func New(cfg Config) (*Node, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		slog.Error("invalid port", slog.Int("port", cfg.Port))
		return nil, fmt.Errorf("invalid port: %d", cfg.Port)
	}

	n := &Node{
		cfg:      cfg,
		selfAddr: fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		uuid:     uuid.New(),
		peers:    peer.NewStore(cfg.PeerLimit, cfg.PeerTimeout),
		seen:     seen.NewSet(),
	}

	slog.Info("node created", slog.String("uuid", n.uuid.String()), slog.String("addr", n.selfAddr))
	return n, nil
}

func (n *Node) Start() error {
	if n.cfg.Bootstrap != "" {
		if err := n.sendHelloToBootstrap(); err != nil {
			return err
		}
	}
	return network.Listen(n.selfAddr, n.handlePacket)
}

func (n *Node) sendHelloTo(addr string) error {
	env, err := message.NewEnvelope(
		message.MessageTypeHello,
		n.uuid.String(),
		n.selfAddr,
		n.cfg.TTL,
		message.HelloPayload{Capabilities: []string{"udp", "json"}},
	)
	if err != nil {
		return fmt.Errorf("create hello: %w", err)
	}
	data, err := env.Encode()
	if err != nil {
		return fmt.Errorf("encode hello: %w", err)
	}
	if err := network.Send(addr, data); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}
	slog.Debug("hello sent", slog.String("to", addr))
	return nil
}

func (n *Node) sendHelloToBootstrap() error {
	if err := n.sendHelloTo(n.cfg.Bootstrap); err != nil {
		return fmt.Errorf("send hello to bootstrap: %w", err)
	}
	slog.Info("hello sent to bootstrap", slog.String("bootstrap", n.cfg.Bootstrap))
	return nil
}

func (n *Node) handlePacket(from *net.UDPAddr, data []byte) {
	env, err := message.DecodeEnvelope(data)
	if err != nil {
		slog.Error("decode envelope failed", slog.String("error", err.Error()))
		return
	}
	fromAddr := ""
	if from != nil {
		fromAddr = from.String()
		if env.SenderID != "" && env.SenderID != n.uuid.String() {
			n.peers.Add(env.SenderID, fromAddr)
		}
	}

	switch env.MsgType {
	case message.MessageTypeHello:
		n.handleHello(fromAddr, env)
	case message.MessageTypeGetPeers:
		n.handleGetPeers(fromAddr, env)
	case message.MessageTypePeers:
		n.handlePeers(fromAddr, env)
	case message.MessageTypeGossip:
		n.handleGossip(fromAddr, env)
	case message.MessageTypePing:
		n.handlePing(fromAddr, env)
	case message.MessageTypePong:
		n.handlePong(fromAddr, env)
	default:
		slog.Warn("unknown message type", slog.String("msg_type", string(env.MsgType)))
	}
}

func (n *Node) handleHello(fromAddr string, env message.Envelope) {
	if fromAddr == "" {
		return
	}
	slog.Info("hello received, replying", slog.String("from", fromAddr), slog.String("sender_id", env.SenderID))
	if err := n.sendHelloTo(fromAddr); err != nil {
		slog.Warn("failed to reply with hello", slog.String("to", fromAddr), slog.String("error", err.Error()))
	}
}

func (n *Node) handleGetPeers(fromAddr string, env message.Envelope) {
	if fromAddr == "" {
		return
	}
	// Decode the payload to get max_peers
	payload, err := message.DecodePayload[message.GetPeersPayload](env)
	if err != nil {
		slog.Warn("failed to decode get_peers payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	// payload is now a message.GetPeersPayload
	peers := n.peers.ListAsPeerInfo()
	// Limit the number of peers to max_peers
	if payload.MaxPeers > 0 && len(peers) > payload.MaxPeers {
		peers = peers[:payload.MaxPeers]
	}
	respPayload := message.PeersListPayload{Peers: peers}
	respEnv, err := message.NewEnvelope(
		message.MessageTypePeers,
		n.uuid.String(),
		n.selfAddr,
		n.cfg.TTL,
		respPayload,
	)
	if err != nil {
		slog.Warn("failed to create peers_list envelope", slog.String("to", fromAddr), slog.String("error", err.Error()))
		return
	}
	data, err := respEnv.Encode()
	if err != nil {
		slog.Warn("failed to encode peers_list envelope", slog.String("to", fromAddr), slog.String("error", err.Error()))
		return
	}
	if err := network.Send(fromAddr, data); err != nil {
		slog.Warn("failed to send peers_list", slog.String("to", fromAddr), slog.String("error", err.Error()))
	} else {
		slog.Info("sent peers_list", slog.String("to", fromAddr), slog.Int("count", len(peers)))
	}
}

func (n *Node) handlePeers(fromAddr string, env message.Envelope) {
	// Decode the payload to get the peer list
	payload, err := message.DecodePayload[message.PeersListPayload](env)
	if err != nil {
		slog.Warn("failed to decode peers_list payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	countAdded := 0
	for _, p := range payload.Peers {
		if p.NodeID == n.uuid.String() {
			continue // Don't add self
		}
		// Try to add peer; Add returns false if already present or limit reached
		if n.peers.Add(p.NodeID, p.Addr) {
			countAdded++
			// Stop if we've reached the peer limit
			if n.peers.Count() >= n.cfg.PeerLimit {
				break
			}
		}
	}
	slog.Info("processed peers_list", slog.String("from", fromAddr), slog.Int("added", countAdded), slog.Int("total", n.peers.Count()))
}

func (n *Node) handleGossip(fromAddr string, env message.Envelope) {
	payload, err := message.DecodePayload[message.GossipPayload](env)
	if err != nil {
		slog.Warn("failed to decode gossip payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	// Dedup key: origin_id + origin_timestamp_ms (unique per logical message)
	msgKey := payload.OriginID + ":" + strconv.FormatInt(payload.OriginTimestampMs, 10)
	if n.seen.Have(msgKey) {
		slog.Debug("gossip already seen, ignoring", slog.String("msg_key", msgKey))
		return
	}
	n.seen.Mark(msgKey)
	slog.Info("gossip received", slog.String("topic", payload.Topic), slog.String("origin_id", payload.OriginID), slog.String("from", fromAddr))

	// Forward to fanout peers if TTL allows (exclude sender)
	if env.TTL > 0 {
		n.forwardGossip(fromAddr, payload, env.TTL-1)
	}
}

func (n *Node) forwardGossip(excludeAddr string, payload message.GossipPayload, ttl int) {
	peers := n.peers.List()
	fanout := n.cfg.Fanout
	if fanout <= 0 {
		fanout = 1
	}
	sent := 0
	for _, p := range peers {
		if sent >= fanout {
			break
		}
		if p.Addr == excludeAddr || p.NodeID == n.uuid.String() {
			continue
		}
		fwdEnv, err := message.NewEnvelope(message.MessageTypeGossip, n.uuid.String(), n.selfAddr, ttl, payload)
		if err != nil {
			slog.Warn("failed to create gossip forward envelope", slog.String("to", p.Addr), slog.String("error", err.Error()))
			continue
		}
		data, err := fwdEnv.Encode()
		if err != nil {
			slog.Warn("failed to encode gossip forward", slog.String("to", p.Addr), slog.String("error", err.Error()))
			continue
		}
		if err := network.Send(p.Addr, data); err != nil {
			slog.Warn("failed to forward gossip", slog.String("to", p.Addr), slog.String("error", err.Error()))
			continue
		}
		sent++
	}
	if sent > 0 {
		slog.Debug("gossip forwarded", slog.Int("count", sent), slog.Int("ttl", ttl))
	}
}

func (n *Node) handlePing(fromAddr string, env message.Envelope) {
	if fromAddr == "" {
		return
	}
	payload, err := message.DecodePayload[message.PingPayload](env)
	if err != nil {
		slog.Warn("failed to decode ping payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	pongPayload := message.PongPayload{PingID: payload.PingID, Seq: payload.Seq}
	pongEnv, err := message.NewEnvelope(message.MessageTypePong, n.uuid.String(), n.selfAddr, n.cfg.TTL, pongPayload)
	if err != nil {
		slog.Warn("failed to create pong envelope", slog.String("to", fromAddr), slog.String("error", err.Error()))
		return
	}
	data, err := pongEnv.Encode()
	if err != nil {
		slog.Warn("failed to encode pong", slog.String("to", fromAddr), slog.String("error", err.Error()))
		return
	}
	if err := network.Send(fromAddr, data); err != nil {
		slog.Warn("failed to send pong", slog.String("to", fromAddr), slog.String("error", err.Error()))
	} else {
		slog.Debug("pong sent", slog.String("to", fromAddr), slog.String("ping_id", payload.PingID))
	}
}

func (n *Node) handlePong(fromAddr string, env message.Envelope) {
	if fromAddr == "" {
		return
	}
	_, err := message.DecodePayload[message.PongPayload](env)
	if err != nil {
		slog.Warn("failed to decode pong payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	// Mark this peer as alive (updates LastSeenAt if already in store)
	n.peers.Add(env.SenderID, fromAddr)
	slog.Debug("pong received, peer alive", slog.String("from", fromAddr), slog.String("sender_id", env.SenderID))
}

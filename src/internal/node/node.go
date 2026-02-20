package node

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

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

	peers        *peer.Store
	seen         *seen.Set
	helloReplied *seen.Set // sender IDs we've already sent a HELLO reply to (avoid infinite loop)
}

func New(cfg Config) (*Node, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		slog.Error("invalid port", slog.Int("port", cfg.Port))
		return nil, fmt.Errorf("invalid port: %d", cfg.Port)
	}

	n := &Node{
		cfg:          cfg,
		selfAddr:     fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		uuid:         uuid.New(),
		peers:        peer.NewStore(cfg.PeerLimit, cfg.PeerTimeout),
		seen:         seen.NewSet(),
		helloReplied: seen.NewSet(),
	}

	slog.Info("node created", slog.String("uuid", n.uuid.String()), slog.String("addr", n.selfAddr))
	return n, nil
}

// Start starts the node: listens for packets and runs the periodic ping loop in the background.
// It returns immediately so the caller can run the CLI or other logic.
func (n *Node) Start() error {
	go func() {
		if err := network.Listen(n.selfAddr, n.handlePacket); err != nil {
			slog.Error("listener exited", slog.String("error", err.Error()))
		}
	}()
	go n.runPingAndPruneLoop()
	if n.cfg.Bootstrap != "" {
		if err := n.sendHelloToBootstrap(); err != nil {
			return err
		}
		if err := n.sendGetPeersToBootstrap(); err != nil {
			return err
		}
	}
	return nil
}

// runPingAndPruneLoop sends PING to all peers at PingInterval and prunes stale peers.
func (n *Node) runPingAndPruneLoop() {
	interval := time.Duration(n.cfg.PingInterval) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		removed := n.peers.PruneStale()
		for _, p := range removed {
			slog.Info("peer removed, no pong received", slog.String("node_id", p.NodeID), slog.String("addr", p.Addr))
		}
		peers := n.peers.List()
		pingID := uuid.New().String()
		for i, p := range peers {
			if p.NodeID == n.uuid.String() {
				continue
			}
			payload := message.PingPayload{PingID: pingID, Seq: int64(i)}
			env, err := message.NewEnvelope(message.MessageTypePing, n.uuid.String(), n.selfAddr, n.cfg.TTL, payload)
			if err != nil {
				slog.Warn("failed to create ping envelope", slog.String("to", p.Addr), slog.String("error", err.Error()))
				continue
			}
			data, err := env.Encode()
			if err != nil {
				slog.Warn("failed to encode ping", slog.String("to", p.Addr), slog.String("error", err.Error()))
				continue
			}
			if err := network.Send(p.Addr, data); err != nil {
				slog.Debug("failed to send ping", slog.String("to", p.Addr), slog.String("error", err.Error()))
			}
		}
	}
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

// sendGetPeersToBootstrap sends GET_PEERS to the bootstrap so we receive PEERS_LIST and fill our peer list up to peer_limit.
func (n *Node) sendGetPeersToBootstrap() error {
	payload := message.GetPeersPayload{MaxPeers: n.cfg.PeerLimit}
	env, err := message.NewEnvelope(message.MessageTypeGetPeers, n.uuid.String(), n.selfAddr, n.cfg.TTL, payload)
	if err != nil {
		return fmt.Errorf("create get_peers: %w", err)
	}
	data, err := env.Encode()
	if err != nil {
		return fmt.Errorf("encode get_peers: %w", err)
	}
	if err := network.Send(n.cfg.Bootstrap, data); err != nil {
		return fmt.Errorf("send get_peers to bootstrap: %w", err)
	}
	slog.Info("get_peers sent to bootstrap", slog.String("bootstrap", n.cfg.Bootstrap), slog.Int("max_peers", n.cfg.PeerLimit))
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
			// Use SenderAddr (where the peer is listening) so we can reach them for gossip/ping.
			// fromAddr is the ephemeral UDP source port and is not where they listen.
			peerAddr := fromAddr
			if env.SenderAddr != "" {
				peerAddr = env.SenderAddr
			}
			n.peers.Add(env.SenderID, peerAddr)
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
	if env.SenderID == "" || env.SenderID == n.uuid.String() {
		return
	}
	// Reply with HELLO only once per sender to avoid infinite HELLO loop.
	if n.helloReplied.Have(env.SenderID) {
		slog.Debug("hello already replied to sender, skipping", slog.String("sender_id", env.SenderID))
		return
	}
	n.helloReplied.Mark(env.SenderID)

	replyTo := fromAddr
	if env.SenderAddr != "" {
		replyTo = env.SenderAddr
	}
	if replyTo == "" {
		return
	}
	slog.Info("hello received, replying", slog.String("from", replyTo), slog.String("sender_id", env.SenderID))
	if err := n.sendHelloTo(replyTo); err != nil {
		slog.Warn("failed to reply with hello", slog.String("to", replyTo), slog.String("error", err.Error()))
	}
}

func (n *Node) handleGetPeers(fromAddr string, env message.Envelope) {
	replyTo := fromAddr
	if env.SenderAddr != "" {
		replyTo = env.SenderAddr
	}
	if replyTo == "" {
		return
	}
	// Decode the payload to get max_peers
	payload, err := message.DecodePayload[message.GetPeersPayload](env)
	if err != nil {
		slog.Warn("failed to decode get_peers payload", slog.String("from", replyTo), slog.String("error", err.Error()))
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
		slog.Warn("failed to create peers_list envelope", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	data, err := respEnv.Encode()
	if err != nil {
		slog.Warn("failed to encode peers_list envelope", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	if err := network.Send(replyTo, data); err != nil {
		slog.Warn("failed to send peers_list", slog.String("to", replyTo), slog.String("error", err.Error()))
	} else {
		slog.Info("sent peers_list", slog.String("to", replyTo), slog.Int("count", len(peers)))
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
	// Log sender by listen address (SenderAddr), not UDP source port (fromAddr)
	from := fromAddr
	if env.SenderAddr != "" {
		from = env.SenderAddr
	}
	slog.Info("gossip received", slog.String("topic", payload.Topic), slog.String("origin_id", payload.OriginID), slog.String("from", from))

	// Forward to fanout peers if TTL allows (exclude sender)
	if env.TTL > 0 {
		n.forwardGossip(fromAddr, payload, env.TTL-1)
	}
}

// PublishGossip starts a new gossip message (e.g. from CLI). It marks the message as seen and sends it to fanout peers.
func (n *Node) PublishGossip(topic string, data string) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		slog.Warn("failed to marshal gossip data", slog.String("error", err.Error()))
		return
	}
	originTs := time.Now().UnixMilli()
	payload := message.GossipPayload{
		Topic:             topic,
		Data:              dataBytes,
		OriginID:          n.uuid.String(),
		OriginTimestampMs: originTs,
	}
	msgKey := payload.OriginID + ":" + strconv.FormatInt(payload.OriginTimestampMs, 10)
	n.seen.Mark(msgKey)
	slog.Info("gossip published", slog.String("topic", topic), slog.String("origin_id", n.uuid.String()))
	n.forwardGossip("", payload, n.cfg.TTL)
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
	payload, err := message.DecodePayload[message.PingPayload](env)
	if err != nil {
		slog.Warn("failed to decode ping payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	// Reply to SenderAddr (where the pinger is listening), not fromAddr (ephemeral UDP port).
	replyTo := env.SenderAddr
	if replyTo == "" {
		replyTo = fromAddr
	}
	pongPayload := message.PongPayload{PingID: payload.PingID, Seq: payload.Seq}
	pongEnv, err := message.NewEnvelope(message.MessageTypePong, n.uuid.String(), n.selfAddr, n.cfg.TTL, pongPayload)
	if err != nil {
		slog.Warn("failed to create pong envelope", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	data, err := pongEnv.Encode()
	if err != nil {
		slog.Warn("failed to encode pong", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	if err := network.Send(replyTo, data); err != nil {
		slog.Warn("failed to send pong", slog.String("to", replyTo), slog.String("error", err.Error()))
	} else {
		slog.Debug("pong sent", slog.String("to", replyTo), slog.String("ping_id", payload.PingID))
	}
}

func (n *Node) handlePong(fromAddr string, env message.Envelope) {
	_, err := message.DecodePayload[message.PongPayload](env)
	if err != nil {
		slog.Warn("failed to decode pong payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	// Mark this peer as alive. Use SenderAddr (where they listen) so we keep the correct address for future pings.
	peerAddr := env.SenderAddr
	if peerAddr == "" {
		peerAddr = fromAddr
	}
	n.peers.Add(env.SenderID, peerAddr)
}

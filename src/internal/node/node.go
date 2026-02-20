package node

import (
	"fmt"
	"log/slog"
	"net"

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
	default:
		// TODO: GET_PEERS, PEERS_LIST, GOSSIP, PING, PONG
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

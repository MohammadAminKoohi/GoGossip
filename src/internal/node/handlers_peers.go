package node

import (
	"fmt"
	"log/slog"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

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
	if err := n.sendWithLog(n.cfg.Bootstrap, data, message.MessageTypeGetPeers); err != nil {
		return fmt.Errorf("send get_peers to bootstrap: %w", err)
	}
	slog.Info("get_peers sent to bootstrap", slog.String("bootstrap", n.cfg.Bootstrap), slog.Int("max_peers", n.cfg.PeerLimit))
	return nil
}

func (n *Node) handleGetPeers(fromAddr string, env message.Envelope) {
	replyTo := fromAddr
	if env.SenderAddr != "" {
		replyTo = env.SenderAddr
	}
	if replyTo == "" {
		return
	}
	payload, err := message.DecodePayload[message.GetPeersPayload](env)
	if err != nil {
		slog.Warn("failed to decode get_peers payload", slog.String("from", replyTo), slog.String("error", err.Error()))
		return
	}
	peers := n.peers.ListAsPeerInfo()
	if payload.MaxPeers > 0 && len(peers) > payload.MaxPeers {
		peers = peers[:payload.MaxPeers]
	}
	respEnv, err := message.NewEnvelope(
		message.MessageTypePeers,
		n.uuid.String(),
		n.selfAddr,
		n.cfg.TTL,
		message.PeersListPayload{Peers: peers},
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
	if err := n.sendWithLog(replyTo, data, message.MessageTypePeers); err != nil {
		slog.Warn("failed to send peers_list", slog.String("to", replyTo), slog.String("error", err.Error()))
	} else {
		slog.Info("sent peers_list", slog.String("to", replyTo), slog.Int("count", len(peers)))
	}
}

func (n *Node) handlePeers(fromAddr string, env message.Envelope) {
	payload, err := message.DecodePayload[message.PeersListPayload](env)
	if err != nil {
		slog.Warn("failed to decode peers_list payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	countAdded := 0
	for _, p := range payload.Peers {
		if p.NodeID == n.uuid.String() {
			continue
		}
		if n.peers.Add(p.NodeID, p.Addr) {
			countAdded++
			if n.peers.Count() >= n.cfg.PeerLimit {
				break
			}
		}
	}
	slog.Info("processed peers_list", slog.String("from", fromAddr), slog.Int("added", countAdded), slog.Int("total", n.peers.Count()))
}

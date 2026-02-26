package node

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

const seenMaxSize = 10_000

func (n *Node) runPingAndPruneLoop(ctx context.Context) {
	interval := time.Duration(n.cfg.PingInterval) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := n.peers.PruneStale()
			for _, p := range removed {
				slog.Info("peer removed, no pong received", slog.String("node_id", p.NodeID), slog.String("addr", p.Addr))
			}
			n.seen.Prune(seenMaxSize)
			n.helloReplied.Prune(seenMaxSize)
			pingID := uuid.New().String()
			for i, p := range n.peers.List() {
				if p.NodeID == n.uuid.String() {
					continue
				}
				env, err := message.NewEnvelope(message.MessageTypePing, n.uuid.String(), n.selfAddr, n.cfg.TTL, message.PingPayload{PingID: pingID, Seq: int64(i)})
				if err != nil {
					slog.Warn("failed to create ping envelope", slog.String("to", p.Addr), slog.String("error", err.Error()))
					continue
				}
				data, err := env.Encode()
				if err != nil {
					slog.Warn("failed to encode ping", slog.String("to", p.Addr), slog.String("error", err.Error()))
					continue
				}
				if err := n.sendWithLog(p.Addr, data, message.MessageTypePing); err != nil {
					slog.Debug("failed to send ping", slog.String("to", p.Addr), slog.String("error", err.Error()))
				}
			}
		}
	}
}

func (n *Node) handlePing(fromAddr string, env message.Envelope) {
	payload, err := message.DecodePayload[message.PingPayload](env)
	if err != nil {
		slog.Warn("failed to decode ping payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}
	replyTo := env.SenderAddr
	if replyTo == "" {
		replyTo = fromAddr
	}
	pongEnv, err := message.NewEnvelope(message.MessageTypePong, n.uuid.String(), n.selfAddr, n.cfg.TTL, message.PongPayload{PingID: payload.PingID, Seq: payload.Seq})
	if err != nil {
		slog.Warn("failed to create pong envelope", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	data, err := pongEnv.Encode()
	if err != nil {
		slog.Warn("failed to encode pong", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	if err := n.sendWithLog(replyTo, data, message.MessageTypePong); err != nil {
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
	peerAddr := env.SenderAddr
	if peerAddr == "" {
		peerAddr = fromAddr
	}
	n.peers.Add(env.SenderID, peerAddr)
}

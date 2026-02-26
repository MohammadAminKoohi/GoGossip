package node

import (
	"context"
	"log/slog"
	"time"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

// runPullLoop periodically advertises known message IDs to a fanout of neighbors (IHAVE),
// enabling them to request any they are missing (IWANT → GOSSIP). It exits when ctx is cancelled.
func (n *Node) runPullLoop(ctx context.Context) {
	interval := time.Duration(n.cfg.PullInterval) * time.Millisecond
	if interval <= 0 {
		return
	}
	maxIds := n.cfg.IHaveMaxIds
	if maxIds <= 0 {
		maxIds = 32
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids := n.gossipCache.ListIDs(maxIds)
			if len(ids) == 0 {
				continue
			}
			peers := n.peers.List()
			if n.cfg.NeighborsPolicy == "random" && len(peers) > 1 {
				n.shufflePeers(peers)
			}
			fanout := n.cfg.Fanout
			if fanout <= 0 {
				fanout = 1
			}
			sent := 0
			for _, p := range peers {
				if sent >= fanout {
					break
				}
				if p.NodeID == n.uuid.String() {
					continue
				}
				env, err := message.NewEnvelope(message.MessageTypeIHave, n.uuid.String(), n.selfAddr, n.cfg.TTL, message.IHavePayload{IDs: ids, MaxIDs: maxIds})
				if err != nil {
					continue
				}
				data, err := env.Encode()
				if err != nil {
					continue
				}
				if err := n.sendWithLog(p.Addr, data, message.MessageTypeIHave); err != nil {
					continue
				}
				sent++
			}
			if sent > 0 {
				slog.Info("IHAVE sent", slog.Int("peers", sent), slog.Int("ids", len(ids)))
			}
		}
	}
}

func (n *Node) handleIHave(fromAddr string, env message.Envelope) {
	replyTo := fromAddr
	if env.SenderAddr != "" {
		replyTo = env.SenderAddr
	}
	if replyTo == "" {
		return
	}
	payload, err := message.DecodePayload[message.IHavePayload](env)
	if err != nil {
		slog.Warn("failed to decode IHAVE payload", slog.String("from", replyTo), slog.String("error", err.Error()))
		return
	}
	var wantIds []string
	for _, id := range payload.IDs {
		if id != "" && !n.seen.Have(id) {
			wantIds = append(wantIds, id)
		}
	}
	if len(wantIds) == 0 {
		return
	}
	iwantEnv, err := message.NewEnvelope(message.MessageTypeIWant, n.uuid.String(), n.selfAddr, n.cfg.TTL, message.IWantPayload{IDs: wantIds})
	if err != nil {
		slog.Warn("failed to create IWANT envelope", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	data, err := iwantEnv.Encode()
	if err != nil {
		slog.Warn("failed to encode IWANT", slog.String("to", replyTo), slog.String("error", err.Error()))
		return
	}
	if err := n.sendWithLog(replyTo, data, message.MessageTypeIWant); err != nil {
		slog.Warn("failed to send IWANT", slog.String("to", replyTo), slog.String("error", err.Error()))
	} else {
		slog.Debug("IWANT sent", slog.String("to", replyTo), slog.Int("count", len(wantIds)))
	}
}

func (n *Node) handleIWant(fromAddr string, env message.Envelope) {
	replyTo := fromAddr
	if env.SenderAddr != "" {
		replyTo = env.SenderAddr
	}
	if replyTo == "" {
		return
	}
	payload, err := message.DecodePayload[message.IWantPayload](env)
	if err != nil {
		slog.Warn("failed to decode IWANT payload", slog.String("from", replyTo), slog.String("error", err.Error()))
		return
	}
	for _, id := range payload.IDs {
		if id == "" {
			continue
		}
		gossipPayload, ok := n.gossipCache.Get(id)
		if !ok {
			continue
		}
		// TTL=0 so the receiver processes and caches it but does not re-forward.
		if err := n.sendGossipTo(replyTo, gossipPayload, 0); err != nil {
			slog.Debug("failed to send GOSSIP for IWANT", slog.String("id", id), slog.String("to", replyTo), slog.String("error", err.Error()))
		}
	}
}

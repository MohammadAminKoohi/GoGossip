package node

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

func (n *Node) handleGossip(fromAddr string, env message.Envelope) {
	payload, err := message.DecodePayload[message.GossipPayload](env)
	if err != nil {
		slog.Warn("failed to decode gossip payload", slog.String("from", fromAddr), slog.String("error", err.Error()))
		return
	}

	msgKey := payload.OriginID + ":" + strconv.FormatInt(payload.OriginTimestampMs, 10)
	if n.seen.Have(msgKey) {
		slog.Debug("gossip already seen, ignoring", slog.String("msg_key", msgKey))
		return
	}
	n.seen.Mark(msgKey)
	n.gossipCache.Add(msgKey, payload)

	n.logExperiment(map[string]interface{}{
		"event":     "gossip_recv",
		"node_id":   n.uuid.String(),
		"origin_id": payload.OriginID,
		"origin_ts": payload.OriginTimestampMs,
		"recv_ms":   time.Now().UnixMilli(),
	})

	from := fromAddr
	if env.SenderAddr != "" {
		from = env.SenderAddr
	}
	if env.TTL == 0 {
		slog.Info("gossip received via pull (IWANT response)", slog.String("topic", payload.Topic), slog.String("origin_id", payload.OriginID), slog.String("from", from))
	} else {
		slog.Info("gossip received", slog.String("topic", payload.Topic), slog.String("origin_id", payload.OriginID), slog.String("from", from))
	}

	if env.TTL > 0 {
		n.forwardGossip(fromAddr, payload, env.TTL-1)
	}
}

// PublishGossip originates a new gossip message from this node (e.g. typed by the user in the CLI).
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
	n.gossipCache.Add(msgKey, payload)

	n.logExperiment(map[string]interface{}{
		"event":     "gossip_publish",
		"node_id":   n.uuid.String(),
		"origin_id": payload.OriginID,
		"origin_ts": payload.OriginTimestampMs,
		"ts_ms":     originTs,
	})

	slog.Info("gossip published", slog.String("topic", topic), slog.String("origin_id", n.uuid.String()))
	n.forwardGossip("", payload, n.cfg.TTL)
}

func (n *Node) forwardGossip(excludeAddr string, payload message.GossipPayload, ttl int) {
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
		if p.Addr == excludeAddr || p.NodeID == n.uuid.String() {
			continue
		}
		env, err := message.NewEnvelope(message.MessageTypeGossip, n.uuid.String(), n.selfAddr, ttl, payload)
		if err != nil {
			slog.Warn("failed to create gossip forward envelope", slog.String("to", p.Addr), slog.String("error", err.Error()))
			continue
		}
		data, err := env.Encode()
		if err != nil {
			slog.Warn("failed to encode gossip forward", slog.String("to", p.Addr), slog.String("error", err.Error()))
			continue
		}
		if err := n.sendWithLog(p.Addr, data, message.MessageTypeGossip); err != nil {
			slog.Warn("failed to forward gossip", slog.String("to", p.Addr), slog.String("error", err.Error()))
			continue
		}
		sent++
	}
	if sent > 0 {
		slog.Debug("gossip forwarded", slog.Int("count", sent), slog.Int("ttl", ttl))
	}
}

// sendGossipTo sends a single GOSSIP message to one address (used for IWANT responses).
func (n *Node) sendGossipTo(addr string, payload message.GossipPayload, ttl int) error {
	env, err := message.NewEnvelope(message.MessageTypeGossip, n.uuid.String(), n.selfAddr, ttl, payload)
	if err != nil {
		return err
	}
	data, err := env.Encode()
	if err != nil {
		return err
	}
	return n.sendWithLog(addr, data, message.MessageTypeGossip)
}

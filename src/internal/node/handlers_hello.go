package node

import (
	"fmt"
	"log/slog"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/pow"
)

func (n *Node) sendHelloTo(addr string) error {
	env, err := message.NewEnvelope(
		message.MessageTypeHello,
		n.uuid.String(),
		n.selfAddr,
		n.cfg.TTL,
		message.HelloPayload{Capabilities: []string{"udp", "json"}, PoW: n.powProof},
	)
	if err != nil {
		return fmt.Errorf("create hello: %w", err)
	}
	data, err := env.Encode()
	if err != nil {
		return fmt.Errorf("encode hello: %w", err)
	}
	if err := n.sendWithLog(addr, data, message.MessageTypeHello); err != nil {
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

func (n *Node) handleHello(fromAddr string, env message.Envelope) {
	if env.SenderID == "" || env.SenderID == n.uuid.String() {
		return
	}

	if n.cfg.PowK > 0 {
		payload, err := message.DecodePayload[message.HelloPayload](env)
		if err != nil || payload.PoW == nil {
			slog.Warn("hello rejected: missing PoW", slog.String("sender_id", env.SenderID))
			return
		}
		p := payload.PoW
		if p.DifficultyK != n.cfg.PowK {
			slog.Warn("hello rejected: wrong difficulty",
				slog.String("sender_id", env.SenderID),
				slog.Int("got", p.DifficultyK),
				slog.Int("want", n.cfg.PowK),
			)
			return
		}
		if !pow.Verify(env.SenderID, p.Nonce, p.DigestHex, n.cfg.PowK) {
			slog.Warn("hello rejected: invalid PoW",
				slog.String("sender_id", env.SenderID),
				slog.Uint64("nonce", p.Nonce),
			)
			return
		}
		slog.Debug("hello PoW valid", slog.String("sender_id", env.SenderID), slog.Uint64("nonce", p.Nonce))
	}

	peerAddr := fromAddr
	if env.SenderAddr != "" {
		peerAddr = env.SenderAddr
	}
	if peerAddr != "" {
		n.peers.Add(env.SenderID, peerAddr)
	}

	// Reply with HELLO only once per sender to avoid an infinite loop.
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

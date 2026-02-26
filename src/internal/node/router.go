package node

import (
	"log/slog"
	"net"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

func (n *Node) handlePacket(from *net.UDPAddr, data []byte) {
	env, err := message.DecodeEnvelope(data)
	if err != nil {
		slog.Error("decode envelope failed", slog.String("error", err.Error()))
		return
	}

	fromAddr := ""
	if from != nil {
		fromAddr = from.String()
		if env.MsgType != message.MessageTypeHello && env.SenderID != "" && env.SenderID != n.uuid.String() {
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
	case message.MessageTypeIHave:
		n.handleIHave(fromAddr, env)
	case message.MessageTypeIWant:
		n.handleIWant(fromAddr, env)
	default:
		slog.Warn("unknown message type", slog.String("msg_type", string(env.MsgType)))
	}
}

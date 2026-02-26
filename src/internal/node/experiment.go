package node

import (
	"time"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/network"
)

// logExperiment writes a JSON metrics line to the experiment log (if configured).
func (n *Node) logExperiment(obj map[string]interface{}) {
	if n.expLog == nil {
		return
	}
	n.expLogMu.Lock()
	defer n.expLogMu.Unlock()
	_ = n.expLog.Encode(obj)
}

// sendWithLog sends data to addr and records a sent-event metric if experiment logging is active.
func (n *Node) sendWithLog(addr string, data []byte, msgType message.MessageType) error {
	if n.expLog != nil {
		n.logExperiment(map[string]interface{}{
			"event":   "msg_sent",
			"node_id": n.uuid.String(),
			"type":    string(msgType),
			"ts_ms":   time.Now().UnixMilli(),
		})
	}
	return network.Send(addr, data)
}

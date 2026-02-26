package node

import "github.com/mohammadaminkoohi/GoGossip/src/internal/peer"

func (n *Node) shufflePeers(peers []*peer.Peer) {
	n.rng.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
}

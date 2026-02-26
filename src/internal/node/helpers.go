package node

import "github.com/mohammadaminkoohi/GoGossip/src/internal/peer"

// shufflePeers randomly reorders a peer slice in-place using the node's local RNG.
func (n *Node) shufflePeers(peers []*peer.Peer) {
	n.rng.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
}

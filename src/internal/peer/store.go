package peer

import (
	"sync"
	"time"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

type Store struct {
	mu      sync.RWMutex
	peers   map[string]*Peer
	limit   int
	timeout time.Duration
}

func NewStore(limit int, timeoutMs int) *Store {
	return &Store{
		peers:   make(map[string]*Peer),
		limit:   limit,
		timeout: time.Duration(timeoutMs) * time.Millisecond,
	}
}

func (s *Store) Add(nodeID, addr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.peers[nodeID]; ok {
		p.Addr = addr
		p.LastSeenAt = time.Now()
		return false
	}
	if s.limit > 0 && len(s.peers) >= s.limit {
		return false
	}
	s.peers[nodeID] = &Peer{NodeID: nodeID, Addr: addr, LastSeenAt: time.Now()}
	return true
}

func (s *Store) Remove(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, nodeID)
}

func (s *Store) RemoveByAddr(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.peers {
		if p.Addr == addr {
			delete(s.peers, id)
			return
		}
	}
}

func (s *Store) List() []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Peer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, &Peer{NodeID: p.NodeID, Addr: p.Addr, LastSeenAt: p.LastSeenAt})
	}
	return out
}

func (s *Store) ListAsPeerInfo() []message.PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]message.PeerInfo, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, message.PeerInfo{NodeID: p.NodeID, Addr: p.Addr})
	}
	return out
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.peers)
}

func (s *Store) PruneStale() {
	if s.timeout <= 0 {
		return
	}
	deadline := time.Now().Add(-s.timeout)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.peers {
		if p.LastSeenAt.Before(deadline) {
			delete(s.peers, id)
		}
	}
}

package seen

import "sync"

// Set tracks message IDs for gossip deduplication.
type Set struct {
	mu sync.RWMutex
	ids map[string]struct{}
}

// NewSet creates an empty seen set.
func NewSet() *Set {
	return &Set{ids: make(map[string]struct{})}
}

// Have returns true if the message ID has been seen.
func (s *Set) Have(msgID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.ids[msgID]
	return ok
}

// Mark records a message ID as seen.
func (s *Set) Mark(msgID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[msgID] = struct{}{}
}

// Prune removes entries so the set has at most maxSize elements (arbitrary eviction).
func (s *Set) Prune(maxSize int) {
	if maxSize <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ids) <= maxSize {
		return
	}
	toDelete := len(s.ids) - maxSize
	for k := range s.ids {
		if toDelete <= 0 {
			break
		}
		delete(s.ids, k)
		toDelete--
	}
}

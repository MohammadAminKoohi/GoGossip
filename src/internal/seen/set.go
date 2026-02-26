package seen

import "sync"

type Set struct {
	mu sync.RWMutex
	ids map[string]struct{}
}

func NewSet() *Set {
	return &Set{ids: make(map[string]struct{})}
}

func (s *Set) Have(msgID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.ids[msgID]
	return ok
}

func (s *Set) Mark(msgID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[msgID] = struct{}{}
}

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

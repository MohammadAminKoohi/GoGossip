package cache

import (
	"sync"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

const DefaultMaxSize = 2000

// GossipCache stores recent gossip payloads keyed by message ID for IWANT responses.
// It maintains insertion order so the most recent IDs can be advertised in IHAVE messages.
// Eviction is LRU by insertion order once the capacity limit is reached.
type GossipCache struct {
	mu    sync.RWMutex
	items map[string]message.GossipPayload
	order []string
	max   int
}

// New creates a GossipCache with the given capacity. If maxSize <= 0 the default is used.
func New(maxSize int) *GossipCache {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	return &GossipCache{
		items: make(map[string]message.GossipPayload),
		max:   maxSize,
	}
}

// Add inserts or refreshes a payload. If the cache is full the oldest entry is evicted.
func (c *GossipCache) Add(id string, payload message.GossipPayload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[id]; ok {
		for i, x := range c.order {
			if x == id {
				c.order = append(append(c.order[:i], c.order[i+1:]...), id)
				break
			}
		}
		return
	}
	if len(c.order) >= c.max {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
	c.items[id] = payload
	c.order = append(c.order, id)
}

// Get returns the payload for the given ID, or false if not present.
func (c *GossipCache) Get(id string) (message.GossipPayload, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.items[id]
	return p, ok
}

// ListIDs returns up to max IDs ordered from most recent to oldest.
func (c *GossipCache) ListIDs(max int) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if max <= 0 || len(c.order) == 0 {
		return nil
	}
	start := len(c.order) - max
	if start < 0 {
		start = 0
	}
	ids := make([]string, 0, max)
	for i := len(c.order) - 1; i >= start && len(ids) < max; i-- {
		ids = append(ids, c.order[i])
	}
	return ids
}

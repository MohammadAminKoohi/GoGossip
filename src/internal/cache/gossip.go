package cache

import (
	"sync"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
)

const DefaultMaxSize = 2000

type GossipCache struct {
	mu    sync.RWMutex
	items map[string]message.GossipPayload
	order []string
	max   int
}

func New(maxSize int) *GossipCache {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	return &GossipCache{
		items: make(map[string]message.GossipPayload),
		max:   maxSize,
	}
}

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

func (c *GossipCache) Get(id string) (message.GossipPayload, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.items[id]
	return p, ok
}

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

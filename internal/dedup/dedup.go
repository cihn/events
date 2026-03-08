package dedup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// GenerateEventID produces a deterministic 32-char hex ID from the natural key.
// Using (event_name, user_id, timestamp) ensures the same logical event always
// maps to the same ID regardless of retry or redelivery.
func GenerateEventID(eventName, userID string, timestamp int64) string {
	raw := fmt.Sprintf("%s\x00%s\x00%d", eventName, userID, timestamp)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16])
}

// Cache is a fixed-capacity FIFO ring buffer for fast in-memory deduplication.
// It tracks the most recent N event IDs. This gives O(1) insert/lookup while
// bounding memory. Older IDs are evicted as new ones arrive; long-term dedup
// is handled by ClickHouse ReplacingMergeTree.
type Cache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]struct{}
	ring     []string
	head     int
}

func NewCache(capacity int) *Cache {
	if capacity <= 0 {
		capacity = 1
	}
	return &Cache{
		capacity: capacity,
		items:    make(map[string]struct{}, capacity),
		ring:     make([]string, capacity),
	}
}

// Check returns true if eventID was already seen. If not, it records the ID,
// evicting the oldest entry when at capacity. The caller can treat true as
// "skip this event."
func (c *Cache) Check(eventID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[eventID]; exists {
		return true
	}

	if evict := c.ring[c.head]; evict != "" {
		delete(c.items, evict)
	}

	c.items[eventID] = struct{}{}
	c.ring[c.head] = eventID
	c.head = (c.head + 1) % c.capacity

	return false
}

func (c *Cache) Remove(eventID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, eventID)
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

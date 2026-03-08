package dedup

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateEventID_Deterministic(t *testing.T) {
	id1 := GenerateEventID("product_view", "user_123", 1723475612)
	id2 := GenerateEventID("product_view", "user_123", 1723475612)
	assert.Equal(t, id1, id2)
	assert.Len(t, id1, 32)
}

func TestGenerateEventID_DifferentInputs(t *testing.T) {
	id1 := GenerateEventID("product_view", "user_123", 1723475612)
	id2 := GenerateEventID("product_view", "user_456", 1723475612)
	id3 := GenerateEventID("purchase", "user_123", 1723475612)
	assert.NotEqual(t, id1, id2)
	assert.NotEqual(t, id1, id3)
}

func TestCache_NewEvent(t *testing.T) {
	c := NewCache(100)
	dup := c.Check("event-1")
	assert.False(t, dup)
}

func TestCache_DuplicateEvent(t *testing.T) {
	c := NewCache(100)
	c.Check("event-1")
	dup := c.Check("event-1")
	assert.True(t, dup)
}

func TestCache_Eviction(t *testing.T) {
	c := NewCache(3)

	c.Check("a")
	c.Check("b")
	c.Check("c")
	assert.Equal(t, 3, c.Len())

	// Should evict "a"
	c.Check("d")
	assert.Equal(t, 3, c.Len())

	// "a" was evicted, so it should not be a duplicate
	dup := c.Check("a")
	assert.False(t, dup)

	// "b" was evicted by "a" insertion, verify "c" is still there
	dup = c.Check("c")
	assert.True(t, dup)
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache(10000)
	var wg sync.WaitGroup

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				c.Check(fmt.Sprintf("event-%d-%d", offset, i))
			}
		}(g)
	}
	wg.Wait()

	require.Equal(t, 10000, c.Len())
}

func TestCache_DuplicatesConcurrent(t *testing.T) {
	c := NewCache(1000)
	var wg sync.WaitGroup
	dupes := make([]int, 10)

	c.Check("shared-event")

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			if c.Check("shared-event") {
				dupes[gid] = 1
			}
		}(g)
	}
	wg.Wait()

	total := 0
	for _, d := range dupes {
		total += d
	}
	assert.Equal(t, 10, total, "all goroutines should see 'shared-event' as duplicate")
}

func TestCache_Remove(t *testing.T) {
	c := NewCache(100)
	c.Check("event-1")
	assert.Equal(t, 1, c.Len())

	c.Remove("event-1")

	dup := c.Check("event-1")
	assert.False(t, dup, "removed event must not be treated as duplicate")
}

func BenchmarkCache_Check(b *testing.B) {
	c := NewCache(1000000)
	for i := 0; i < b.N; i++ {
		c.Check(fmt.Sprintf("event-%d", i))
	}
}

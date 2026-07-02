package sentiment

import (
	"sync"
	"time"
)

type Cache struct {
	mu      sync.RWMutex
	signal  *SentimentSignal
	history []SentimentSignal
}

func NewCache() *Cache {
	return &Cache{}
}

func (c *Cache) Set(s SentimentSignal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := s
	c.signal = &copy
	c.history = append([]SentimentSignal{copy}, c.history...)
	if len(c.history) > 5 {
		c.history = c.history[:5]
	}
}

func (c *Cache) History() []SentimentSignal {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]SentimentSignal, len(c.history))
	copy(out, c.history)
	return out
}

func (c *Cache) Get() (SentimentSignal, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.signal == nil {
		return SentimentSignal{}, false
	}
	return *c.signal, true
}

func (c *Cache) Current(now time.Time) (SentimentSignal, bool) {
	s, ok := c.Get()
	if !ok {
		return SentimentSignal{}, false
	}
	if !s.IsValid(now) {
		return SentimentSignal{}, false
	}
	return s, true
}

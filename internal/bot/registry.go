package bot

import (
	"fmt"
	"sort"
	"sync"
)

type registration struct {
	meta    Meta
	factory Factory
}

var (
	mu    sync.RWMutex
	registry = map[string]registration{}
)

func Register(meta Meta, factory Factory) {
	if meta.ID == "" || factory == nil {
		panic("bot: invalid registration")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[meta.ID]; exists {
		panic("bot: duplicate registration " + meta.ID)
	}
	registry[meta.ID] = registration{meta: meta, factory: factory}
}

func RegisteredIDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func AllMeta() []Meta {
	ids := RegisteredIDs()
	out := make([]Meta, 0, len(ids))
	for _, id := range ids {
		out = append(out, Describe(id))
	}
	return out
}

func Describe(id string) Meta {
	mu.RLock()
	reg, ok := registry[id]
	mu.RUnlock()
	if !ok {
		return Meta{ID: id}
	}
	return reg.meta
}

func New(id string, deps *Deps) (Bot, error) {
	mu.RLock()
	reg, ok := registry[id]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown bot %q (registered: %v)", id, RegisteredIDs())
	}
	return reg.factory(deps)
}

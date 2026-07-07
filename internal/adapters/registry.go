package adapters

import (
	"fmt"
	"sort"
	"sync"

	"github.com/zinan-c/Poised/internal/core"
)

type Registry struct {
	mutex    sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

func (registry *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}
	if adapter.Name() == "" {
		return fmt.Errorf("adapter name is empty")
	}

	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	if _, exists := registry.adapters[adapter.Name()]; exists {
		return fmt.Errorf("adapter %q already registered", adapter.Name())
	}

	registry.adapters[adapter.Name()] = adapter
	return nil
}

func (registry *Registry) Get(name string) (Adapter, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	adapter, exists := registry.adapters[name]
	return adapter, exists
}

func (registry *Registry) List() []core.AdapterInfo {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	infos := make([]core.AdapterInfo, 0, len(registry.adapters))
	for _, adapter := range registry.adapters {
		infos = append(infos, core.AdapterInfo{
			Name: adapter.Name(),
			Kind: adapter.Kind(),
		})
	}

	sort.Slice(infos, func(leftIndex int, rightIndex int) bool {
		return infos[leftIndex].Name < infos[rightIndex].Name
	})

	return infos
}

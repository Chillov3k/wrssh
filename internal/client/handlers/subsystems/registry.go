package subsystems

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module
}

func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]Module)}
}

var defaultRegistry = NewRegistry()

func Register(module Module) {
	if err := defaultRegistry.Register(module); err != nil {
		panic(err)
	}
}

func Lookup(name string) (Module, bool) {
	return defaultRegistry.Lookup(name)
}

func Manifests() []Manifest {
	return defaultRegistry.Manifests()
}

func Names() []string {
	return defaultRegistry.Names()
}

func (r *Registry) Register(module Module) error {
	if module == nil {
		return fmt.Errorf("nil module")
	}
	manifest := module.Manifest()
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		return fmt.Errorf("module name is required")
	}
	if strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("module name %q contains whitespace", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("module %q already registered", name)
	}
	r.modules[name] = module
	return nil
}

func (r *Registry) Lookup(name string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	module, ok := r.modules[strings.TrimSpace(name)]
	return module, ok
}

func (r *Registry) Manifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Manifest, 0, len(r.modules))
	for _, module := range r.modules {
		result = append(result, module.Manifest())
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (r *Registry) Names() []string {
	manifests := r.Manifests()
	names := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		names = append(names, manifest.Name)
	}
	return names
}

package providers

import (
	"fmt"
	"strings"
)

type Registry struct {
	defaultProvider string
	adapters        map[string]Adapter
	ordered         []Adapter
}

func NewRegistry(defaultProvider string, adapters ...Adapter) (*Registry, error) {
	registry := &Registry{
		defaultProvider: strings.TrimSpace(defaultProvider),
		adapters:        make(map[string]Adapter, len(adapters)),
		ordered:         make([]Adapter, 0, len(adapters)),
	}

	for _, adapter := range adapters {
		if adapter == nil {
			return nil, fmt.Errorf("provider adapter cannot be nil")
		}
		providerID := strings.TrimSpace(adapter.ProviderID())
		if providerID == "" {
			return nil, fmt.Errorf("provider adapter returned an empty provider id")
		}
		if _, exists := registry.adapters[providerID]; exists {
			return nil, fmt.Errorf("provider %q is registered more than once", providerID)
		}

		registry.adapters[providerID] = adapter
		registry.ordered = append(registry.ordered, adapter)
	}

	if len(registry.adapters) == 0 {
		return nil, fmt.Errorf("at least one provider adapter is required")
	}
	if registry.defaultProvider == "" {
		return nil, fmt.Errorf("default provider is required")
	}
	if _, ok := registry.adapters[registry.defaultProvider]; !ok {
		return nil, fmt.Errorf("default provider %q is not registered", registry.defaultProvider)
	}

	return registry, nil
}

func (r *Registry) DefaultProviderID() string {
	if r == nil {
		return ""
	}
	return r.defaultProvider
}

func (r *Registry) ProviderIDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.ordered))
	for _, adapter := range r.ordered {
		ids = append(ids, adapter.ProviderID())
	}
	return ids
}

func (r *Registry) Resolve(providerID string) (Adapter, error) {
	if r == nil {
		return nil, fmt.Errorf("provider registry is not configured")
	}

	selected := strings.TrimSpace(providerID)
	if selected == "" {
		selected = r.defaultProvider
	}

	adapter, ok := r.adapters[selected]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q (supported: %s)", selected, strings.Join(r.ProviderIDs(), ", "))
	}
	return adapter, nil
}

func (r *Registry) ResolveRunURL(providerID, raw string) (Adapter, *RunLocator, error) {
	if r == nil {
		return nil, nil, fmt.Errorf("provider registry is not configured")
	}

	if explicit := strings.TrimSpace(providerID); explicit != "" {
		adapter, err := r.Resolve(explicit)
		if err != nil {
			return nil, nil, err
		}
		locator, err := adapter.ParseRunURL(raw)
		if err != nil {
			return nil, nil, err
		}
		return adapter, locator, nil
	}

	if len(r.ordered) == 1 {
		adapter := r.ordered[0]
		locator, err := adapter.ParseRunURL(raw)
		if err != nil {
			return nil, nil, err
		}
		return adapter, locator, nil
	}

	for _, adapter := range r.ordered {
		locator, err := adapter.ParseRunURL(raw)
		if err == nil {
			return adapter, locator, nil
		}
	}

	return nil, nil, fmt.Errorf("run_url did not match any configured provider (supported: %s)", strings.Join(r.ProviderIDs(), ", "))
}

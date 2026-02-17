package adapter

import (
	"errors"
	"fmt"
	"sync"
)

// adapterFactories holds registered adapter constructors.
var (
	adapterFactories []func() Adapter
	adapterMu        sync.RWMutex
)

// RegisterFactory registers an adapter constructor.
func RegisterFactory(factory func() Adapter) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapterFactories = append(adapterFactories, factory)
}

// DetectAdapters scans for available adapters for the given project.
// Returns all successfully detected adapters and an aggregated error
// if any Detect() calls failed.
func DetectAdapters(projectRoot string) (map[string]Adapter, error) {
	adapterMu.RLock()
	factories := make([]func() Adapter, len(adapterFactories))
	copy(factories, adapterFactories)
	adapterMu.RUnlock()

	adapters := make(map[string]Adapter)
	var errs []error

	for _, factory := range factories {
		instance := factory()
		detected, err := instance.Detect(projectRoot)
		if err != nil {
			errs = append(errs, fmt.Errorf("adapter %s: %w", instance.ID(), err))
			continue
		}
		if !detected {
			continue
		}
		if _, exists := adapters[instance.ID()]; exists {
			errs = append(errs, fmt.Errorf("duplicate adapter ID: %s", instance.ID()))
			continue
		}
		adapters[instance.ID()] = instance
	}
	return adapters, errors.Join(errs...)
}

// AllAdapters creates all registered adapter instances without filtering by Detect.
// Use this at startup so that all adapters are available across project switches;
// consumers (e.g. conversations plugin) call Detect() per-adapter to filter by project.
func AllAdapters() map[string]Adapter {
	adapterMu.RLock()
	factories := make([]func() Adapter, len(adapterFactories))
	copy(factories, adapterFactories)
	adapterMu.RUnlock()

	adapters := make(map[string]Adapter, len(factories))
	for _, factory := range factories {
		instance := factory()
		adapters[instance.ID()] = instance
	}
	return adapters
}

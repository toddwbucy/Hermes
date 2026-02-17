package amp

import "github.com/toddwbucy/hermes/internal/adapter"

func init() {
	adapter.RegisterFactory(func() adapter.Adapter {
		return New()
	})
}

package ports

import "github.com/serverkraken/flow/internal/domain"

// ConfigReader returns the resolved worktime configuration (file + env
// merged, defaults applied). One read per invocation — adapters that
// cache do so internally.
type ConfigReader interface {
	Load() (domain.Config, error)
}

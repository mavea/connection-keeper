package manager

import (
	"connection-keeper/internal/domain"
	"time"
)

type config struct {
	connectionCheckInterval time.Duration
	disableReadinessTimeout time.Duration
	stopTimeout             time.Duration
}

// NewConf создает runtime-конфигурацию цикла менеджера.
func NewConf(connectionCheckInterval, disableReadinessTimeout, stopTimeout time.Duration) domain.RunConfig {
	return &config{
		connectionCheckInterval: connectionCheckInterval,
		disableReadinessTimeout: disableReadinessTimeout,
		stopTimeout:             stopTimeout,
	}
}

func (c *config) ConnectionCheckInterval() time.Duration {
	return c.connectionCheckInterval
}

func (c *config) DisableReadinessTimeout() time.Duration {
	return c.disableReadinessTimeout
}

func (c *config) StopTimeout() time.Duration {
	return c.stopTimeout
}

package main

import "time"

// ── конфигурация дренажа ────────────────────────────────────────────────────

type drainConfig struct{}

func (drainConfig) CancelWaitTimeOut() time.Duration { return 200 * time.Millisecond }
func (drainConfig) MaxRetryWaitAttempts() uint8      { return 3 }

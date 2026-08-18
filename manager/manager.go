package manager

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"connection-keeper/internal/kind"
	"connection-keeper/internal/lifecycle"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

var (
	ErrCtxCancel               = errors.New("context canceled")
	ErrCheckIntervalIsNotValid = errors.New("connection check interval must be > 0")
	ErrManagerAlreadyRunning   = errors.New("manager already running")
	ErrManagerIsNotStopped     = errors.New("manager is not stopped")
	ErrLoggerIsNil             = errors.New("logger is nil")
)

type manager struct {
	ctx          context.Context
	kindMgr      intlDomain.KindManager
	connectorMgr intlDomain.ConnectorManager
	registryMgr  intlDomain.RegistryManager
	drainMgr     intlDomain.DrainManager

	readiness atomic.Bool
	conf      intlDomain.RunConfig
	logger    domain.Logger

	run       context.Context
	runCancel context.CancelFunc
	done      chan struct{}
	singleRun atomic.Bool
}

type ManagerConfig interface {
	intlDomain.DrainConfig
}

func New(
	ctx context.Context,
	conf ManagerConfig,
) intlDomain.Manager {
	return newMgr(
		ctx,
		kind.NewKindManager(),
		lifecycle.NewConnectorManager(),
		lifecycle.NewRegistryManager(),
		lifecycle.NewDrainManager(conf),
		slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	)
}

func newMgr(
	ctx context.Context,
	kindMgr intlDomain.KindManager,
	connectorMgr intlDomain.ConnectorManager,
	registryMgr intlDomain.RegistryManager,

	drainMgr intlDomain.DrainManager,
	logger domain.Logger,
) *manager {
	mgr := &manager{
		ctx:          ctx,
		kindMgr:      kindMgr,
		connectorMgr: connectorMgr,
		registryMgr:  registryMgr,
		drainMgr:     drainMgr,

		readiness: atomic.Bool{},

		logger:    logger,
		singleRun: atomic.Bool{},
	}

	return mgr
}

func (mgr *manager) stop() error {
	if mgr.runCancel != nil {
		c := mgr.runCancel
		mgr.runCancel = nil
		c()
		timerStop := time.NewTimer(mgr.conf.StopTimeout())
		defer timerStop.Stop()

		select {
		case <-mgr.done:
		case <-timerStop.C:
			return ErrManagerIsNotStopped
		}
	}

	return nil
}

func (mgr *manager) shutdownDisableReadiness() error {
	mgr.DisableReadiness()
	timer := time.NewTimer(mgr.conf.DisableReadinessTimeout())
	defer timer.Stop()

	select {
	case <-mgr.run.Done():
		return errors.Join(ErrCtxCancel, mgr.run.Err())
	case <-timer.C:
	}
	return nil
}
func (mgr *manager) shutdownConnectors() error {
	var (
		connector intlDomain.Connector
		err       error
		errs      error
	)

	for _, connector = range mgr.connectorMgr.List() {
		if connector == nil {
			continue
		}
		err = connector.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

func (mgr *manager) Shutdown() error {
	if mgr.conf == nil {
		return nil
	}
	err := mgr.shutdownDisableReadiness()
	if err != nil {
		return err
	}
	err = mgr.stop()
	if err != nil {
		return err
	}
	return mgr.shutdown()
}

func (mgr *manager) shutdown() error {
	var errs error
	err := mgr.shutdownConnectors()
	if err != nil {
		errs = errors.Join(errs, err)
	}

	if err = mgr.DrainManager().DrainAll(); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

func (mgr *manager) KindManager() intlDomain.KindManager {
	return mgr.kindMgr
}
func (mgr *manager) ConnectorManager() intlDomain.ConnectorManager {
	return mgr.connectorMgr
}
func (mgr *manager) DrainManager() intlDomain.DrainManager {
	return mgr.drainMgr
}
func (mgr *manager) RegistryManager() intlDomain.RegistryManager {
	return mgr.registryMgr
}
func (mgr *manager) SetLogger(logger domain.Logger) error {
	if logger == nil {
		return ErrLoggerIsNil
	}
	mgr.logger = logger

	return nil
}
func (mgr *manager) Logger() domain.Logger {
	return mgr.logger
}

func (mgr *manager) Readiness() bool {
	return mgr.readiness.Load()
}
func (mgr *manager) SetReadiness(value bool) (oldValue bool) {
	return mgr.readiness.Swap(value)
}
func (mgr *manager) DisableReadiness() (oldValue bool) {
	return mgr.readiness.Swap(false)
}
func (mgr *manager) EnableReadiness() (oldValue bool) {
	return mgr.readiness.Swap(true)
}

func (mgr *manager) Run(ctx context.Context, conf intlDomain.RunConfig) error {
	if !mgr.singleRun.CompareAndSwap(false, true) {
		return ErrManagerAlreadyRunning
	}
	defer func() {
		mgr.singleRun.Store(false)
	}()

	var (
		err          error
		connectorMgr = mgr.connectorMgr

		newInterval = conf.ConnectionCheckInterval()
		interval    = newInterval
		timer       = time.NewTimer(interval)
		updateCheck bool
	)

	if mgr.runCancel != nil {
		return ErrManagerAlreadyRunning
	}
	mgr.run, mgr.runCancel = context.WithCancel(ctx)
	mgr.conf = conf
	mgr.done = make(chan struct{})
	defer func() {
		if mgr.runCancel != nil {
			mgr.runCancel = nil
		}
		mgr.conf = nil
		var ok bool
		select {
		case _, ok = <-mgr.done:
			if !ok {
				close(mgr.done)
			}
		default:
			close(mgr.done)
		}

		timer.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return ErrCtxCancel
		case <-mgr.run.Done():
			return nil
		case <-timer.C:
			newInterval = conf.ConnectionCheckInterval()
			if newInterval != interval {
				if newInterval <= 0 {
					return ErrCheckIntervalIsNotValid
				}
				interval = newInterval
			}
		}

		updateCheck = true
		for updateCheck {
			if updateCheck, err = mgr.checkAndUpdateRegistry(ctx, connectorMgr); err != nil {
				_ = mgr.DisableReadiness()
				errs := mgr.shutdown()
				if errs != nil {
					return errs
				}
				return err
			}
		}

		select {
		case <-ctx.Done():
			return ErrCtxCancel
		case <-mgr.run.Done():
			return nil
		default:
			mgr.EnableReadiness()
		}

		if err = mgr.DrainManager().DrainNext(); err != nil {
			mgr.logger.WarnContext(ctx, fmt.Sprintf("drain next error: %v", err))
		}

		timer.Reset(interval)
	}
}

func (mgr *manager) checkAndUpdateRegistry(
	ctx context.Context,
	connectorMgr intlDomain.ConnectorManager,
) (bool, error) {
	var (
		updateCheck  bool
		updatesCheck bool
		connector    intlDomain.Connector
		err          error
	)
	for _, connector = range connectorMgr.List() {
		if connector == nil {
			continue
		}
		updateCheck, err = connector.ReconnectIfNeeded(ctx, false)
		if err != nil {
			return false, err
		}
		updatesCheck = updatesCheck || updateCheck
	}

	return updatesCheck, nil
}

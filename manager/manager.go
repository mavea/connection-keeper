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
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCtxCancel               = errors.New("context canceled")
	ErrCheckIntervalIsNotValid = errors.New("connection check interval must be > 0")
	ErrManagerAlreadyRunning   = errors.New("manager already running")
	ErrManagerAlreadyStopped   = errors.New("manager already stopped")
	ErrManagerIsNotStopped     = errors.New("manager is not stopped")
	ErrLoggerIsNil             = errors.New("logger is nil")
)

type managerRun struct {
	run    atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
}

func newManagerRun() *managerRun {
	return &managerRun{
		run: atomic.Bool{},
		mu:  sync.Mutex{},
	}
}

func (mgrR *managerRun) isRunning() bool {
	return mgrR.run.Load()
}
func (mgrR *managerRun) Run(ctx context.Context, cancelFunc context.CancelFunc) error {
	mgrR.mu.Lock()
	defer mgrR.mu.Unlock()
	if mgrR.isRunning() {
		return ErrManagerAlreadyRunning
	}
	mgrR.ctx = ctx
	mgrR.cancel = cancelFunc
	mgrR.done = make(chan struct{})
	if !mgrR.run.CompareAndSwap(false, true) {
		return ErrManagerAlreadyRunning
	}

	return nil
}
func (mgrR *managerRun) Done() <-chan struct{} {
	return mgrR.done
}
func (mgrR *managerRun) Ctx() context.Context {
	return mgrR.ctx
}

func (mgrR *managerRun) Stop() error {
	mgrR.mu.Lock()
	defer mgrR.mu.Unlock()
	if !mgrR.run.CompareAndSwap(true, false) {
		return ErrManagerAlreadyStopped
	}
	if mgrR.cancel != nil {
		mgrR.cancel()
	}
	mgrR.cancel = nil
	return nil
}

type manager struct {
	ctx          context.Context
	kindMgr      intlDomain.KindManager
	connectorMgr intlDomain.ConnectorManager
	registryMgr  intlDomain.RegistryManager
	drainMgr     intlDomain.DrainManager

	readiness atomic.Bool
	conf      intlDomain.RunConfig
	logger    domain.Logger

	run *managerRun
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

		logger: logger,
		run:    newManagerRun(),
	}

	return mgr
}

func (mgr *manager) stop() error {

	if mgr.run.Stop() != nil {
		timerStop := time.NewTimer(mgr.conf.StopTimeout())
		defer timerStop.Stop()

		select {
		case <-mgr.run.Done():
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
	case <-mgr.run.Ctx().Done():
		return errors.Join(ErrCtxCancel, mgr.run.Ctx().Err())
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
	var (
		err          error
		connectorMgr = mgr.connectorMgr

		newInterval = conf.ConnectionCheckInterval()
		interval    = newInterval
		timer       = time.NewTimer(interval)
		updateCheck bool
	)
	if err = mgr.run.Run(context.WithCancel(ctx)); err != nil {
		return err
	}
	mgr.conf = conf
	defer func() {
		mgr.conf = nil
		var ok bool
		select {
		case _, ok = <-mgr.run.done:
			if !ok {
				close(mgr.run.done)
			}
		default:
			close(mgr.run.done)
		}

		timer.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return ErrCtxCancel
		case <-mgr.run.Ctx().Done():
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
		case <-mgr.run.Ctx().Done():
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

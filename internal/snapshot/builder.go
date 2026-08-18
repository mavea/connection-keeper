package snapshot

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"errors"
	"sync"
)

var (
	ErrInvalidSnapshotConnectionType = errors.New("invalid snapshot connection type")
)

type builder struct {
	registryMgr      intlDomain.RegistryManager
	capturedReleases []domain.CancelFunc
	mu               sync.Mutex
	conns            sync.Map
}

func newBuilder(mgr intlDomain.Manager) *builder {
	return &builder{
		registryMgr:      mgr.RegistryManager(),
		mu:               sync.Mutex{},
		capturedReleases: make([]domain.CancelFunc, 0),
		conns:            sync.Map{},
	}
}

func (b *builder) Connection(kind domain.Kind) (any, error) {
	connOld, bl := b.conns.Load(kind.ID())
	if !bl {
		gen := b.registryMgr.GetGeneration(kind)
		conn, release := gen.Any()
		conn, bl = b.conns.LoadOrStore(kind.ID(), conn)
		if bl {
			// Если другой поток уже создал соединение, закрываем своё.
			// Это безопасно, так как release-функция гарантирует корректное закрытие соединения.
			err := release()
			if err != nil {
				return nil, err
			}
		} else {
			// Если мы создали соединение, сохраняем release-функцию.
			b.mu.Lock()
			defer b.mu.Unlock()

			b.capturedReleases = append(b.capturedReleases, release)
		}
		return conn, nil
	}
	return connOld, nil
}

func (b *builder) release() error {
	var err, errs error
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, release := range b.capturedReleases {
		err = release()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	b.capturedReleases = nil

	return errs
}

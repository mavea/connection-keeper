package main

import (
	"connection-keeper/connector"
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"connection-keeper/manager"
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

func Query(gen intlDomain.Generation[dbClient]) {
	conn, release := gen.Conn()
	if conn == nil {
		log.Printf("[app#%d] соединение не доступно", gen.Version())
		return
	}
	defer func() {
		if err := release(); err != nil {
			log.Printf("[app#%d] ошибка release: %v", gen.Version(), err)
		}
	}()
	log.Printf("[app#%d] refs: %d", conn.Version(), gen.Refs())
}

func main() {
	ctxGlobal, cancel := context.WithCancel(context.Background())
	defer cancel()

	dsn := "mock://localhost/mydb"
	startedAt := time.Now()
	updateAt := startedAt.Add(3 * time.Second)
	var updateOnce atomic.Bool

	keeper := manager.New(ctxGlobal, drainConfig{})

	// Регистрируем dbClient как Output-коннектор.
	// Output — соединение, инициируемое приложением (БД, HTTP-клиент исходящих запросов, продюсер очереди).
	dbKind, err := connector.NewOutput[dbClient](
		ctxGlobal,
		"mock-db",
		keeper,
		connector.NewConfig(
			3, time.Second, 100*time.Millisecond, // readiness: retries, timeout, interval
			3, 100*time.Millisecond, // connect: retries, interval
			false, 0, // disable-readiness on update: off
		),
		// ConnectionInvalidationFunc: проверяет, нужно ли пересоздать соединение.
		// Возвращает true один раз после того, как пройдёт 3 секунды.
		func() (bool, error) {
			if updateOnce.Load() {
				return false, nil
			}
			if time.Now().Before(updateAt) {
				return false, nil
			}
			if updateOnce.CompareAndSwap(false, true) {
				log.Println("[db] обнаружена необходимость обновления соединения")
				return true, nil
			}
			return false, nil
		},
		// CreateOutputConnectionFunc: создаёт новое соединение и возвращает его lifecycle-колбэки.
		func(ctx context.Context) (*dbClient, domain.ReadinessFunc, domain.ShutdownConnectionFunc, error) {
			db := newDb(dsn)
			return db,
				// ReadinessFunc: проверяет готовность соединения (аналог SELECT 1).
				func(ctx context.Context) error {
					log.Printf("[db#%d] проверка готовности соединения\n", db.Version())
					return nil
				},
				// ShutdownConnectionFunc: начинает остановку соединения при смене поколения.
				// Возвращает DrainingFunc, которая будет вызвана после снятия всех удержаний.
				func(ctx context.Context) (domain.DrainingFunc, error) {
					log.Printf("[db#%d] остановка соединения\n", db.Version())
					return func() error {
						db.Close()
						log.Printf("[db#%d] дренаж соединения завершён\n", db.Version())
						return nil
					}, nil
				},
				nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	// ── горутина бизнес-логики ───────────────────────────────────────────────
	// Каждые 700 мс выполняет «запрос к БД» через текущее поколение соединения.
	//
	// Для получения соединения используется GetGeneration + type assertion к
	// intlDomain.Generation[dbClient]. В будущем это место займёт публичный GetSnapshot API.
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(700 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctxGlobal.Done():
				return
			case <-tick.C:
				log.Println("[app] вызов к БД")
				genAny := keeper.RegistryManager().GetGeneration(dbKind)
				if genAny == nil {
					log.Println("[app] соединение ещё не установлено")
					continue
				}
				gen, ok := genAny.(intlDomain.Generation[dbClient])
				if !ok {
					log.Println("[app] неожиданный тип поколения")
					continue
				}
				Query(gen)
			}
		}
	}()

	// ── горутина менеджера ───────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[app] connection-keeper запущен")
		if err := keeper.Run(ctxGlobal, manager.NewConf(2*time.Second, time.Second, 2*time.Second)); err != nil {
			log.Printf("[app] connection-keeper завершился: %v", err)
		}
	}()

	log.Println("[app] сервис запущен")
	time.Sleep(7 * time.Second)

	log.Println("[app] получен сигнал остановки, завершение...")
	if err := keeper.Shutdown(); err != nil {
		log.Printf("[app] ошибка при завершении: %v", err)
	}
	cancel()

	wg.Wait()
	log.Println("[app] сервис остановлен")
}

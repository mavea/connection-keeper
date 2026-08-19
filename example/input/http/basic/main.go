package main

import (
	"connection-keeper/connector"
	"connection-keeper/domain"
	"connection-keeper/manager"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type drainConfig struct{}

func (d drainConfig) CancelWaitTimeOut() time.Duration { return 100 * time.Millisecond }
func (d drainConfig) MaxRetryWaitAttempts() uint8      { return 3 }

func main() {
	resChan := make(chan struct{}, 1)
	cancelChan := make(chan struct{}, 1)
	first := true
	ctxGlobal := context.Background()
	connectionKeeper := manager.New(ctxGlobal, drainConfig{})
	count := atomic.Uint64{}
	wg := sync.WaitGroup{}
	wg.Add(2)
	_, err := connector.NewInput(
		ctxGlobal,
		"http",
		connectionKeeper,
		connector.NewConfig(
			3,
			time.Millisecond*200,
			time.Millisecond*100,
			3,
			time.Millisecond*100,
			false,
			time.Second,
		),
		func() (bool, error) {
			select {
			case <-resChan:
				return true, nil
			default:
				return false, nil
			}
		},
		func(ctx context.Context) (*http.Server, domain.ReadinessFunc, domain.HandoverConnectionFunc[http.Server], error) {
			log.Println("создание коннекта")
			num := count.Add(1)
			log.Printf("запуск сервера-%d", num)
			mux := http.NewServeMux()
			mux.HandleFunc("/api/print", func(w http.ResponseWriter, r *http.Request) {
				if _, err := fmt.Fprintf(w, "сервер пинг-%d\n", num); err != nil {
					log.Printf("ошибка ответа /api/print: %v", err)
				}
			})
			mux.HandleFunc("/api/change", func(w http.ResponseWriter, r *http.Request) {
				resChan <- struct{}{}
				if _, err := fmt.Fprintf(w, "сервер смена-%d\n", num); err != nil {
					log.Printf("ошибка ответа /api/change: %v", err)
				}
			})
			mux.HandleFunc("/api/end", func(w http.ResponseWriter, r *http.Request) {
				cancelChan <- struct{}{}
				if _, err := fmt.Fprintf(w, "сервер смена-%d\n", num); err != nil {
					log.Printf("ошибка ответа /api/end: %v", err)
				}
			})

			server := &http.Server{
				Addr: ":8080",
			}
			server.Handler = mux
			if first {
				first = false

				go func() {
					log.Println("первичный сервер запускается")
					if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						log.Println("ошибка сервера:", err)
					}
				}()
			}

			return server,
				func(ctx context.Context) error {
					return nil
				},
				func(ctx context.Context, next *http.Server) (domain.DrainingFunc, error) {
					log.Println("ресурс передан во вторичный сервер")

					ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
					defer cancel()

					if err := server.Shutdown(ctx); err != nil {
						log.Println("ошибка остановки сервера:", err)
					}
					if next != nil {
						go func() {
							log.Println("вторичный сервер запускается")
							if err := next.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
								log.Println("ошибка сервера:", err)
							}
						}()
					}

					return func() error {
						return nil
					}, nil
				}, nil
		},
	)

	if err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Println("запуск connection-keeper")
		errL := connectionKeeper.Run(ctxGlobal, manager.NewConf(time.Second*5, time.Second, time.Second))
		if errL != nil {
			log.Fatal(errL)
		}

		wg.Done()
	}()
	go func() {
		select {
		case <-cancelChan:
		case <-ctxGlobal.Done():
		}
		errL := connectionKeeper.Shutdown()
		if errL != nil {
			log.Fatal(errL)
		}
		wg.Done()
	}()
	log.Println("Всё запущено")

	wg.Wait()
	log.Println("Завершение работы сервиса")
}

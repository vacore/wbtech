package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

/*
Программа должна корректно завершаться по нажатию Ctrl+C (SIGINT).
Выберите и обоснуйте способ завершения работы всех горутин-воркеров при получении сигнала прерывания.
Подсказка: можно использовать контекст (context.Context) или канал для оповещения о завершении.
*/

func main() {
	// Prefer context instead of notification channel
	// It is the standard and idiomatic way, used across the ecosystem.
	// Has additional goodies (WithTimeout(), WithValue(), etc.)
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT) // route SIGINT to sigChan, if it happens

	var wg sync.WaitGroup
	numWorkers := 3

	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done(): // check if need to stop
					fmt.Printf("Worker[%d] received ctx.Done\n", id)
					return
				default: // proceed with default action (some work)
					fmt.Printf("Worker[%d]: working...\n", id)
					time.Sleep(1 * time.Second)
				}
			}
		}(i)
	}

	sig := <-sigChan // block in waiting for Ctrl+C (SIGINT)
	fmt.Printf("Received signal: %v\n", sig)

	cancel() // closes the ctx.Done() channel (unblocks the return case in workers)

	wg.Wait()
	fmt.Println("All workers done. Exiting...")
}

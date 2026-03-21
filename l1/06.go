package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

/*
Реализовать все возможные способы остановки выполнения горутины.
Классические подходы: выход по условию, через канал уведомления, через контекст, прекращение работы runtime.Goexit() и др.
Продемонстрируйте каждый способ в отдельном фрагменте кода.
*/

func main() {
	demoContextCancel()
	demoContextTimeout()
	demoCloseInput()
	demoNaturalReturn()
	demoMultiClose()
	demoDoneSend()
	demoMultiCancel()
	demoAtomicFlag()
	demoSyncCond()
	demoRuntimeGoexit()
	demoPanicRecover()
	demoMainExit()
}

// 1. Context with cancel - the standard way
func demoContextCancel() {
	fmt.Println("1. Method: context.WithCancel")

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		<-ctx.Done()
		fmt.Printf("   1: cancelled by context (err = %v)\n", ctx.Err())
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	wg.Wait()
	fmt.Println("   1: goroutine stopped\n")
}

// 2. Context with timeout - also standard, a variation of #1
func demoContextTimeout() {
	fmt.Println("2. Method: context.WithTimeout")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		<-ctx.Done()
		fmt.Printf("   2: returned by timeout (err = %v)\n", ctx.Err())
	}()

	wg.Wait()
	fmt.Println("   2: goroutine stopped\n")
}

// 3. Close input channel - another canonical pattern
func demoCloseInput() {
	fmt.Println("3. Method: close input channel")

	ch := make(chan int, 5)
	var wg sync.WaitGroup
	wg.Add(1)

	// Consumer
	go func() {
		defer wg.Done()

		for v := range ch { // exits when ch is closed AND drained
			fmt.Printf("   3: consumer got %d\n", v)
		}
		fmt.Println("   3: input channel closed")
	}()

	// Producer
	for i := 1; i <= 5; i++ {
		ch <- i
	}
	close(ch) // We're done producing, close consumer

	wg.Wait()
	fmt.Println("   3: goroutine stopped\n")
}

// 4. Natural return - simple and reliable
func demoNaturalReturn() {
	fmt.Println("4. Method: natural return")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 5; i++ {
			fmt.Printf("   4: step %d\n", i)
		}
		fmt.Println("   4: done, returning naturally")
	}()

	wg.Wait()
	fmt.Println("   4: goroutine stopped\n")
}

// 5. Broadcast via channel close() - similar to context but weaker
func demoMultiClose() {
	fmt.Println("5. Method: broadcast via channel close")

	quit := make(chan struct{})
	const N = 5

	var wg sync.WaitGroup
	wg.Add(N)

	for id := 0; id < N; id++ {
		go func(id int) {
			defer wg.Done()

			<-quit // blocks until quit is closed
			fmt.Printf("   5: goroutine %d received quit\n", id)
		}(id)
	}

	time.Sleep(100 * time.Millisecond)
	close(quit) // broadcast to all goroutines waiting on this channel

	wg.Wait()
	fmt.Println("   5: all goroutines stopped\n")
}

// 6. Single send via channel - works for exactly one goroutine.
func demoDoneSend() {
	fmt.Println("6. Method: send via channel")

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		<-done
		fmt.Println("   6: received done signal")
	}()

	time.Sleep(100 * time.Millisecond)
	done <- struct{}{} // send one signal - only one receiver gets it

	wg.Wait()
	fmt.Println("   6: goroutine stopped\n")
}

// 7. Multiple cancellation sources - use where signal priority groups needed
func demoMultiCancel() {
	fmt.Println("7. Method: multiple cancellation sources")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hardKill := make(chan struct{})
	workCh := make(chan int)

	var wg sync.WaitGroup
	wg.Add(1)

	// Worker
	go func() {
		defer wg.Done()

		for {
			// First phase: non-blocking check for cancellation or hard-kill
			select {
			case <-ctx.Done():
				fmt.Println("   7: worker: context cancelled (priority path)")
				return
			case <-hardKill:
				fmt.Println("   7: worker: hard-killed")
				return
			default:
			}

			// Second phase: block on work or cancellation
			select {
			case <-ctx.Done():
				fmt.Println("   7: worker: context cancelled")
				return
			case <-hardKill:
				fmt.Println("   7: worker: hard-killed")
				return
			case v := <-workCh:
				fmt.Printf("   7: worker: processed %d\n", v)
			}
		}
	}()

	// Send some work, then hard-kill
	workCh <- 1
	workCh <- 2
	workCh <- 3

	close(hardKill)
	wg.Wait()
	fmt.Println("   7: goroutine stopped\n")
}

// 8. Polling an atomic flag - C-style, may be used for performance
func demoAtomicFlag() {
	fmt.Println("8. Method: polling atomic flag")

	var stop atomic.Bool // initially false
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; !stop.Load(); i++ { // wait for the flag to exit
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Println("   8: goroutine: saw stop flag")
	}()

	time.Sleep(200 * time.Millisecond)
	stop.Store(true) // change the flag

	wg.Wait()
	fmt.Println("   8: goroutine stopped\n")
}

// 9. Conditional variable - C-style, may be used for broadcast wakeup with a complex predicate
func demoSyncCond() {
	fmt.Println("9. Method: sync.Cond")

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	shouldStop := false

	const N = 3
	var wg sync.WaitGroup
	wg.Add(N)

	for id := 0; id < N; id++ {
		go func(id int) {
			defer wg.Done()

			mu.Lock()
			for !shouldStop { // re-check guard
				cond.Wait() // atomically unlocks mu and suspends
			}
			mu.Unlock()

			fmt.Printf("   9: goroutine %d: woke up, stopping\n", id)
		}(id)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	shouldStop = true
	mu.Unlock()
	cond.Broadcast() // wake ALL waiters

	wg.Wait()
	fmt.Println("   9: all goroutines stopped\n")
}

// 10. runtime.Goexit() - terminates the goroutine but still runs deferred functions.
func demoRuntimeGoexit() {
	fmt.Println("10. Method: runtime.Goexit()")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer fmt.Println("   10: deferred cleanup")

		fmt.Println("   10: calling runtime.Goexit()")
		runtime.Goexit()

		// This line is NEVER reached.
		fmt.Println("   10: this is dead code")
	}()

	wg.Wait()
	fmt.Println("   10: goroutine stopped\n")
}

// 11. panic/recover - the goroutine exits via panic; recover prevents program crash.
func demoPanicRecover() {
	fmt.Println("11. Method: panic/recover")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("   11: recovered from panic: %v\n", r)
			}
		}()

		for i := 0; i < 5; i++ {
			if i == 3 {
				panic("stop at i==3") // goroutine exits
			}
			fmt.Printf("   11: working, i=%d\n", i)
		}
	}()

	wg.Wait()
	fmt.Println("   11: goroutine stopped\n")
}

// 12. Main exit - returns or calls os.Exit, all goroutines are killed immediately
func demoMainExit() {
	fmt.Println("12. Method: os.Exit() kills all goroutines")

	// Spawn a long-lived goroutine
	go func() {
		for {
			time.Sleep(time.Second)
		}
	}()

	fmt.Println("   12: long-lived goroutine spawned; calling os.Exit(0) now")
	fmt.Println("   12: all goroutines (including the long-lived one) will be gone")
	os.Exit(0)
}

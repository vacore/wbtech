package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

/*
Реализовать постоянную запись данных в канал (в главной горутине).
Реализовать набор из N воркеров, которые читают данные из этого канала и выводят их в stdout.
Программа должна принимать параметром количество воркеров и при старте создавать указанное число горутин-воркеров.
*/

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <N>\n", os.Args[0])
		os.Exit(1)
	}

	n, err := strconv.Atoi(os.Args[1])
	if err != nil || n <= 0 {
		fmt.Fprintf(os.Stderr, "Number of workers must be a positive number!\n")
		os.Exit(1)
	}

	var wg sync.WaitGroup
	ch := make(chan int, n) // buffered channel with size n

	for i := 1; i <= n; i++ {
		wg.Add(1)

		go func(id int, ch <-chan int) {
			defer wg.Done()

			for val := range ch {
				fmt.Printf("Worker[%d] fetched data: %d\n", id, val)
			}

			fmt.Printf("Worker[%d] done\n", id)
		}(i, ch)
	}

	fmt.Printf("%d workers started\n", n)

	for i := 1; i <= 16; i++ {
		fmt.Printf("Sending data into channel: %d\n", i)
		ch <- i
		time.Sleep(200 * time.Millisecond)
	}

	close(ch)
	wg.Wait()

	fmt.Println("All workers are done!")
}

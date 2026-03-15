package main

import (
	"fmt"
	"time"
)

/*
Разработать программу, которая будет последовательно отправлять значения в канал,
а с другой стороны канала – читать эти значения. По истечении N секунд программа должна завершаться.
Подсказка: используйте time.After или таймер для ограничения времени работы.
*/

const N = 5 * time.Second

func main() {
	ch := make(chan int) // channel for passing values

	// Goroutine, sequentially sending values into channel
	go func() {
		defer close(ch)
		timeout := time.After(N)

		workPause := time.NewTimer(0) // Duration - 0, fires right away
		defer workPause.Stop()

	loop:
		for i := 0; ; i++ {
			select {
			case <-timeout:
				break loop
			case <-workPause.C:
			}

			select {
			case <-timeout:
				break loop
			case ch <- i:
				fmt.Printf("Sender  : %d -> ch\n", i)
			}

			workPause.Reset(time.Second)
		}

		fmt.Println("Sender stopping.")
	}()

	// Main goroutine will read values from the channel
	for val := range ch {
		fmt.Printf("Receiver: ch -> %d\n", val)
	}
	fmt.Println("Receiver stopping.")

	fmt.Println("Program done.")
}

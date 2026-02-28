package main

import (
	"fmt"
	"sync"
)

/*
Написать программу, которая конкурентно рассчитает значения квадратов чисел, взятых из массива [2,4,6,8,10], и выведет результаты в stdout.
Подсказка: запусти несколько горутин, каждая из которых возводит число в квадрат.
*/

type Squared struct {
	num     int
	squared int
}

func main() {
	squares := []Squared{
		{num: 2},
		{num: 4},
		{num: 6},
		{num: 8},
		{num: 10},
	}

	var wg sync.WaitGroup

	for i := range squares {
		wg.Add(1)
		go func(sq *Squared) {
			defer wg.Done()
			sq.squared = sq.num * sq.num
		}(&squares[i])
	}

	wg.Wait()

	for _, sq := range squares {
		fmt.Printf("num = %d, squared = %d\n", sq.num, sq.squared)
	}
}

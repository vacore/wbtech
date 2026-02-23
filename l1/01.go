package main

import "fmt"

/*
Дана структура Human (с произвольным набором полей и методов).
Реализовать встраивание методов в структуре Action от родительской структуры Human (аналог наследования).
Подсказка: используйте композицию (embedded struct), чтобы Action имел все методы Human.
*/

type Human struct {
	age    uint
	height uint
}

type Action struct {
	Human
}

func (h *Human) Grow(cm uint) {
	h.height += cm
}

func (h *Human) Olden(years uint) {
	h.age += years
}

func (h Human) String() string {
	return fmt.Sprintf("age: %d, height: %d", h.age, h.height)
}

func (a *Action) Run(m uint) {
	fmt.Printf("Running %d meters!\n", m)
}

func main() {
	human := Human{age: 25, height: 180}
	fmt.Println(human)

	human.Olden(1)
	human.Grow(3)
	fmt.Println(human)

	action := Action{Human: human}
	action.Olden(2) // human's method
	action.Grow(5)  // human's method
	fmt.Println(action)
	action.Run(100) // action's method
}

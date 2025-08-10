package main

import "fmt"

func printSequre(n int) {
	fmt.Print(n*n, " ")
}
func main() {
	//第一种格式
	fmt.Println("第一种格式(标准for循环):")
	for i := 0; i < 10; i++ {
		printSequre(i)
	}
	fmt.Println()

	//第二种格式
	fmt.Println("第二种格式(带条件变量):")
	i := 0
	for ok := true; ok; ok = (i != 10) {
		printSequre(i)
		i++
	}
	fmt.Println()

	//第三种格式
	fmt.Println("第三种格式(无限循环+break)")
	i = 0
	for {
		if i == 10 {
			break
		}
		printSequre(i)
		i++
	}
	fmt.Println()

	//第四种range
	fmt.Println("第四种格式(range遍历):")
	aSlice := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	for _, v := range aSlice {
		printSequre(v)
	}
}

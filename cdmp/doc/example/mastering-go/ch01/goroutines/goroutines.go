package main

import (
	"fmt"
	"sync"
)

func myPrint(start, end int, wait *sync.WaitGroup) {

	for i := start; i < end; i++ {
		fmt.Print(i, " ")
	}
	fmt.Println()
}

func main() {
	var wait sync.WaitGroup
	var mu sync.Mutex
	fmt.Println("开始第一轮.......")
	const loops = 10
	for i := 0; i < loops; i++ {
		wait.Add(1)

		go func(base int) {
			defer wait.Done() //保持代码统一
			mu.Lock()
			defer mu.Unlock()
			myPrint(base, loops, &wait)
		}(i)
	}
	wait.Wait()

	fmt.Println("第一轮循环结束,开始第二轮")
	for i := 0; i < loops; i++ {
		wait.Add(1)
		go func(base int) {
			defer wait.Done()
			mu.Lock()
			defer mu.Unlock()
			for j := loops; j >= base; j-- {
				fmt.Print(j, " ")
			}
			fmt.Println()
		}(i)
	}
	wait.Wait()
	fmt.Println("第二轮循环结束")

}

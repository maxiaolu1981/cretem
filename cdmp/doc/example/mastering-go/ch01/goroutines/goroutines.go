// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

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

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

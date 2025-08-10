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
	"os"
	"strconv"
)

func main() {
	args := os.Args

	if len(args) != 2 {
		fmt.Println("用法:程序名 <整数参数>")
		os.Exit(1)
	}

	arg1, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Printf("%s必须是整数\n", os.Args[1])
		return
	}
	switch arg1 {
	case 0:
		fmt.Println("Zero")
	case 1:
		fmt.Println("One")
	case 2, 3, 4:
		fallthrough //直接打印值
	default:
		fmt.Println("值:", arg1)
	}

	switch {
	case arg1 > 0:
		fmt.Println("正数")
	case arg1 == 0:
		fmt.Println("零")
	case arg1 < 0:
		fmt.Println("负数")
	}

}

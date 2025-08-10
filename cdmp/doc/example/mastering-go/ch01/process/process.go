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
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("用法:程序<参数1><参数2>[可选参数...]")
		os.Exit(1)
	}
	var (
		totalValid  int
		intCount    int
		floatCount  int
		inValidArgs []string
	)
	for _, arg := range args {
		if _, err := strconv.Atoi(arg); err == nil {
			intCount++
			totalValid++
			continue
		}
		if _, err := strconv.ParseFloat(arg, 64); err == nil {
			floatCount++
			totalValid++
			continue
		}
		inValidArgs = append(inValidArgs, arg)
	}
	fmt.Println("参数统计结果:")
	fmt.Println("总有效参数:", totalValid)
	fmt.Println("整数统计:", intCount)
	fmt.Println("浮点统计:", floatCount)
	fmt.Println("无效参数", len(inValidArgs))
	if len(inValidArgs) > totalValid {
		fmt.Println("输入无效参数太多...")
		for i, v := range inValidArgs {
			fmt.Printf("无效参数%d:%q\n", i+1, v)
		}
	}
}

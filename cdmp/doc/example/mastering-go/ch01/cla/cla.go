package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	args := os.Args
	if len(args) < 2 {
		fmt.Println("用法: 程序名<参数><参数2> [可选参数...]")
		os.Exit(1)
	}
	var min, max float64
	for i, arg := range args[1:] {
		result, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			fmt.Printf("警告:参数%s无法解释为数字,已经跳过\n", arg)
			continue
		}
		if i == 1 {
			min, max = result, result
		}
		if min > result {
			min = result
		}
		if max < result {
			max = result
		}
	}
	fmt.Println("min=", min, "max=", max)
}

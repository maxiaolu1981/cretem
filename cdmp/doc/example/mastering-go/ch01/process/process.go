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

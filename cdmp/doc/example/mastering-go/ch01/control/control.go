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

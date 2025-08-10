package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("请输入你的名字:")
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			fmt.Printf("%v输入错误", err)
			return
		} else {
			fmt.Println("输入被中断,退出.")
			return
		}
	}
	name := strings.TrimSpace(scanner.Text())
	if name == "" {
		fmt.Println("姓名不能为空")
		return
	}
	fmt.Println("hi:", name)

}

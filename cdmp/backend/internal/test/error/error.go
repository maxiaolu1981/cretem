package main

import (
	"fmt"
	"io"
	"os"

	base "github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func main() {
	// if err := getDatabase(); err != nil {
	// 	//if base.IsCode(err, code.ErrTokenInvalid) {
	// 	fmt.Printf("找到了错误%+v\n", err)
	// 	//}
	// 	//coder := base.ParseCode(err)
	// 	//fmt.Printf("%+v\n", coder)
	// 	//	base.ListAllCodes()

	// }
	writer := &ConsoleWriter{
		Prefix: "[日志]",
	}
	_, err := fmt.Fprintf(writer, "这是一条测试数据")
	if err != nil {
		fmt.Println("写入失败", err)
	}
}

func getDatabase() error {
	if err := getUser(); err != nil {
		return base.WrapC(err, 100, "验证失败.")
	}
	return nil
}

func getUser() error {
	if err := queryDatabase(); err != nil {
		return base.WrapC(err, 1, "正确.")
	}
	return nil
}

func queryDatabase() error {
	return base.WrapC(fmt.Errorf("asdsf"), 10, "密码无效")
}

func writeData(w io.Writer, data string) error {
	_, err := w.Write([]byte(data))
	return err
}

type ConsoleWriter struct {
	Prefix string
}

func (c *ConsoleWriter) Write(p []byte) (int, error) {
	_, err := os.Stdout.WriteString(c.Prefix)
	if err != nil {
		return 0, err
	}
	n, err := os.Stdout.Write(p)
	return n, err
}

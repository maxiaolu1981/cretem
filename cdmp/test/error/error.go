package main

import (
	"fmt"

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	base "github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func main() {
	if err := getDatabase(); err != nil {
		//if base.IsCode(err, code.ErrTokenInvalid) {
		fmt.Printf("找到了错误%+v\n", err)
		//}
		//coder := base.ParseCode(err)
		//fmt.Printf("%+v\n", coder)
		//	base.ListAllCodes()
	}
}

func getDatabase() error {
	if err := getUser(); err != nil {
		return base.WrapC(err, code.ErrDatabase, "验证失败.")
	}
	return nil
}

func getUser() error {
	if err := queryDatabase(); err != nil {
		return base.WrapC(err, code.ErrSuccess, "正确.")
	}
	return nil
}

func queryDatabase() error {
	return base.WrapC(fmt.Errorf("asdsf"), code.ErrPasswordIncorrect, "密码无效")
}

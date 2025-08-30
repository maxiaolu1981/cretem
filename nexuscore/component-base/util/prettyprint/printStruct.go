package prettyprint

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/tabwriter"
)

// Print 一键打印结构体（包括嵌套结构体）
func Print(title string, data interface{}) error {
	result, err := PrintStruct(title, data)
	if err != nil {
		return err
	}

	// 直接输出到标准输出
	fmt.Print(result)
	return nil
}

// printStruct 核心打印逻辑
func PrintStruct(title string, data interface{}) (string, error) {
	if data == nil {
		return fmt.Sprintf("=== %s ===\n<nil>\n", title), nil
	}

	// 获取终端宽度
	cols := getTerminalWidth()
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// 写入标题
	writeTitle(tw, title, cols)

	// 递归打印结构体
	if err := printRecursive(tw, data, "", 0); err != nil {
		return "", err
	}

	// 刷新输出
	if err := tw.Flush(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// printRecursive 递归打印
func printRecursive(tw *tabwriter.Writer, data interface{}, prefix string, depth int) error {
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			fmt.Fprintf(tw, "%s<nil>\n", prefix)
			return nil
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		fmt.Fprintf(tw, "%s%v\n", prefix, val.Interface())
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		if !field.CanInterface() {
			continue
		}

		fieldName := getFieldName(fieldType)
		fullPrefix := prefix + fieldName + ":"

		if isNestedStruct(field) {
			fmt.Fprintf(tw, "%s\n", fullPrefix)
			if err := printRecursive(tw, field.Interface(), prefix+"  ", depth+1); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(tw, "%s\t%v\n", fullPrefix, field.Interface())
		}
	}

	return nil
}

// getFieldName 获取字段名
func getFieldName(field reflect.StructField) string {
	if jsonTag := field.Tag.Get("json"); jsonTag != "" {
		if name := strings.Split(jsonTag, ",")[0]; name != "" && name != "-" {
			return name
		}
	}
	if mapTag := field.Tag.Get("mapstructure"); mapTag != "" {
		return mapTag
	}
	return field.Name
}

// isNestedStruct 判断嵌套结构体
func isNestedStruct(field reflect.Value) bool {
	kind := field.Kind()
	if kind == reflect.Ptr {
		if field.IsNil() {
			return false
		}
		field = field.Elem()
		kind = field.Kind()
	}
	return kind == reflect.Struct
}

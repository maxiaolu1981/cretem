package main

import (
	"log"
	"log/syslog"
)

func main() {
	syslog, err := syslog.New(syslog.LOG_USER|syslog.LOG_INFO, "myapp")
	if err != nil {
		log.Printf("无法连接到syslog服务%v\n", err)
		return
	}
	defer func() {
		err := syslog.Close()
		if err != nil {
			log.Printf("关闭syslog失%v\n", err)
			return
		}
	}()
	log.SetOutput(syslog)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("程序启动成功")
	log.Printf("处理请求完成")

}

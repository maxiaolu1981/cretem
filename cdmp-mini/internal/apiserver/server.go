package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
)

// NewApp主要功能是初始化日志、数据库连接，并通过数据层查询用户信息，完整展示了从环境初始化到具体业务操作的流程
func NewApp(basename string) {
	//生成日志参数
	opts := options.NewOptions()

	// //初始化日志
	// log.Init(opts)
	// //确保程序退出时刷新日志缓冲区，避免日志丢失
	// log.Flush()
	// //定义mysql options
	// mysqlOptions := options.NewMySQLOptions()
	// //得到mysql包实例:GetMySQLFactoryOr
	// storeIns, err := mysql.GetMySQLFactoryOr(mysqlOptions)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// //得到上下文
	// ctx := context.Background()
	// //GetOptions初始化
	// getOptions := metav1.GetOptions{
	// 	TypeMeta: metav1.TypeMeta{
	// 		Kind:       "User",
	// 		APIVersion: "v1",
	// 	},
	// }

	// //// 调用用户查询方法：通过数据工厂获取用户操作实例，查询用户名为 "admin" 的用户
	// user, err := storeIns.Users().Get(ctx, "admin", getOptions)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(user)
}

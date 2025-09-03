
curl -X POST "http://127.0.0.1:8080/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@2021"}'


./get_user.sh "令牌" 100 50 --并发数100,每个线程50次请求
# 批量创建1000个用户
./create_user.sh batch-1000

单条删除
./delete_single_user.sh "你的JWT令牌" "test-user123"
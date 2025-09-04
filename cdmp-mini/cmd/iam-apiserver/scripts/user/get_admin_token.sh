#!/bin/bash

# 配置参数
BASE_URL="http://localhost:8080"  # API基础地址，根据实际情况修改
LOGIN_ENDPOINT="/v1/auth/login"   # 登录接口路径，根据实际情况修改
ADMIN_USERNAME="admin"            # 管理员用户名
ADMIN_PASSWORD="Admin@2021"       # 管理员密码
TOKEN_FILE="./admin_token.txt"    # token保存路径

# 颜色输出函数
green() { echo -e "\033[32m$1\033[0m"; }
red() { echo -e "\033[31m$1\033[0m"; }
blue() { echo -e "\033[34m$1\033[0m"; }

blue "=== 开始获取管理员token ==="
blue "登录地址: $BASE_URL$LOGIN_ENDPOINT"
blue "用户名: $ADMIN_USERNAME"

# 发送登录请求并获取token
response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL$LOGIN_ENDPOINT" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "'"$ADMIN_USERNAME"'",
    "password": "'"$ADMIN_PASSWORD"'"
  }')

# 提取响应体和状态码
http_body=$(echo "$response" | head -n -1)
http_code=$(echo "$response" | tail -n 1)

# 检查HTTP状态码
if [ "$http_code" -ne 200 ]; then
    red "❌ 登录失败，HTTP状态码: $http_code"
    red "响应内容: $http_body"
    exit 1
fi

# 提取token（假设响应格式为 {"code":0,"data":{"token":"xxx"},"message":"success"}）
# 根据实际响应格式调整jq的解析路径
token=$(echo "$http_body" | jq -r '.data.token')

if [ "$token" = "null" ] || [ -z "$token" ]; then
    red "❌ 无法从响应中提取token"
    red "响应内容: $http_body"
    exit 1
fi

# 保存token到文件
echo "$token" > "$TOKEN_FILE"

green "✅ 管理员token获取成功"
green "token值: $token"
green "已保存到: $(realpath $TOKEN_FILE)"
green "使用方式: 在请求头中添加 Authorization: Bearer $(cat $TOKEN_FILE)"

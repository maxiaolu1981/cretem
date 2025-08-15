#!/bin/bash
# API服务验证脚本：检查服务状态、登录及接口访问
# 使用方法：chmod +x validate-api.sh && ./validate-api.sh

# 配置服务地址（根据实际情况修改）
BASE_URL="http://localhost:8080"
USERNAME="testuser"   # 替换为实际用户名
PASSWORD="Test@123"   # 替换为实际密码
TOKEN=""              # 用于存储登录后的令牌

# 打印分隔线
print_sep() {
  echo "=============================================="
}

# 1. 验证服务是否存活（健康检查）
print_sep
echo "1. 验证服务是否存活: ${BASE_URL}/healthz"
response=$(curl -s -w "%{http_code}" "${BASE_URL}/healthz")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n -1)

if [ "$http_code" -eq 200 ]; then
  echo "✅ 服务正常运行"
else
  echo "❌ 服务未启动或健康检查失败（状态码: $http_code）"
  exit 1
fi

# 2. 验证登录接口（获取JWT令牌）
print_sep
echo "2. 验证登录接口: ${BASE_URL}/login"
login_response=$(curl -s -w "%{http_code}" -X POST "${BASE_URL}/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\": \"${USERNAME}\", \"password\": \"${PASSWORD}\"}")

login_http_code=$(echo "$login_response" | tail -n1)
login_body=$(echo "$login_response" | head -n -1)

if [ "$login_http_code" -eq 200 ]; then
  # 从响应中提取token（假设响应格式为{"token":"xxx"}）
  TOKEN=$(echo "$login_body" | grep -o '"token":"[^"]*' | cut -d'"' -f4)
  if [ -n "$TOKEN" ]; then
    echo "✅ 登录成功，令牌已获取"
  else
    echo "❌ 登录响应中未找到token（响应: $login_body）"
    exit 1
  fi
else
  echo "❌ 登录失败（状态码: $login_http_code，响应: $login_body）"
  exit 1
fi

# 3. 验证受保护接口（使用令牌访问用户列表）
print_sep
echo "3. 验证受保护接口: ${BASE_URL}/v1/users"
protected_response=$(curl -s -w "%{http_code}" "${BASE_URL}/v1/users" \
  -H "Authorization: Bearer ${TOKEN}")

protected_http_code=$(echo "$protected_response" | tail -n1)
protected_body=$(echo "$protected_response" | head -n -1)

if [ "$protected_http_code" -eq 200 ]; then
  echo "✅ 受保护接口访问成功"
  echo "接口响应预览: $(echo "$protected_body" | jq '.items | length' 2>/dev/null) 个用户"
else
  echo "❌ 受保护接口访问失败（状态码: $protected_http_code，响应: $protected_body）"
  exit 1
fi

# 4. 验证404路由处理
print_sep
echo "4. 验证404路由: ${BASE_URL}/invalid-path"
notfound_response=$(curl -s -w "%{http_code}" "${BASE_URL}/invalid-path")
notfound_http_code=$(echo "$notfound_response" | tail -n1)

if [ "$notfound_http_code" -eq 404 ]; then
  echo "✅ 404路由处理正常（状态码: $notfound_http_code）"
else
  echo "❌ 404路由处理异常（状态码: $notfound_http_code）"
  exit 1
fi

print_sep
echo "🎉 所有API验证通过！"


#!/bin/bash

# 测试配置
BASE_URL="http://localhost:8080/v1/users"
# 替换为有效的管理员token
ADMIN_TOKEN="your_admin_token_here"
# 替换为有效的普通用户token
USER_TOKEN="your_user_token_here"
# 生成一个随机用户名，避免重复
TEST_USERNAME="test_user_$(date +%s)"
TEST_EMAIL="test_$(date +%s)@example.com"

# 颜色输出函数
green() { echo -e "\033[32m$1\033[0m"; }
red() { echo -e "\033[31m$1\033[0m"; }
yellow() { echo -e "\033[33m$1\033[0m"; }
blue() { echo -e "\033[34m$1\033[0m"; }

# 测试结果统计
total=0
passed=0
failed=0

# 测试函数
run_test() {
    local test_name=$1
    local description=$2
    local command=$3
    local expected_code=$4
    local expected_message=$5

    total=$((total + 1))
    blue "\n=== 测试 $total: $test_name ==="
    echo "描述: $description"
    echo "执行命令: $command"
    
    # 执行测试命令并捕获输出
    result=$(eval $command)
    http_code=$(echo "$result" | grep -oP '(?<="http_status":)[0-9]+' | head -n 1)
    response_code=$(echo "$result" | grep -oP '(?<="code":)[0-9]+' | head -n 1)
    message=$(echo "$result" | grep -oP '(?<="message":")[^"]+' | head -n 1)
    
    # 显示响应信息
    echo "HTTP状态码: $http_code"
    echo "响应码: $response_code"
    echo "响应消息: $message"
    
    # 验证结果
    if [ "$http_code" -eq "$expected_code" ] && [[ "$message" == *"$expected_message"* ]]; then
        green "✅ 测试通过"
        passed=$((passed + 1))
    else
        red "❌ 测试失败"
        echo "预期HTTP状态码: $expected_code"
        echo "预期消息包含: $expected_message"
        failed=$((failed + 1))
    fi
}

# 1. 测试使用正确参数创建用户（成功场景）
run_test \
    "使用正确参数创建用户" \
    "发送合法的用户创建请求，应返回201状态码和成功消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d '{
        \"name\": \"'$TEST_USERNAME'\",
        \"email\": \"'$TEST_EMAIL'\",
        \"password\": \"ValidPass123!\",
        \"nickname\": \"Test User\",
        \"phone\": \"13800138000\"
    }'" \
    201 \
    "用户创建成功"

# 2. 测试创建已存在的用户（冲突场景）
run_test \
    "创建已存在的用户" \
    "尝试创建用户名已存在的用户，应返回409状态码和冲突消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d '{
        \"name\": \"'$TEST_USERNAME'\",
        \"email\": \"another_'$TEST_EMAIL'\",
        \"password\": \"ValidPass123!\",
        \"nickname\": \"Duplicate User\",
        \"phone\": \"13900139000\"
    }'" \
    409 \
    "已存在"

# 3. 测试参数绑定失败（缺少必填字段）
run_test \
    "参数绑定失败（缺少必填字段）" \
    "发送缺少用户名的请求，应返回400状态码和绑定失败消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d '{
        \"email\": \"missing_username@example.com\",
        \"password\": \"ValidPass123!\"
    }'" \
    400 \
    "参数绑定失败"

# 4. 测试用户名不合法（格式错误）
run_test \
    "用户名不合法" \
    "发送包含特殊字符的用户名，应返回422状态码和验证错误消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d '{
        \"name\": \"invalid@user.name\",
        \"email\": \"invalid_username@example.com\",
        \"password\": \"ValidPass123!\"
    }'" \
    422 \
    "用户名不合法"

# 5. 测试密码不符合规则（强度不够）
run_test \
    "密码不符合规则" \
    "发送弱密码请求，应返回422状态码和密码验证错误消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d '{
        \"name\": \"weakpassworduser\",
        \"email\": \"weak_password@example.com\",
        \"password\": \"123\"
    }'" \
    422 \
    "密码不符合规则"

# 6. 测试未提供Authorization头
run_test \
    "未提供Authorization头" \
    "不发送认证令牌，应返回401状态码和未授权消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -d '{
        \"name\": \"noauthtestuser\",
        \"email\": \"no_auth@example.com\",
        \"password\": \"ValidPass123!\"
    }'" \
    401 \
    "缺少Authorization头"

# 7. 测试使用无效token
run_test \
    "使用无效token" \
    "发送无效的认证令牌，应返回401状态码和认证失败消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer invalid_token' \
    -d '{
        \"name\": \"invalidtokentest\",
        \"email\": \"invalid_token@example.com\",
        \"password\": \"ValidPass123!\"
    }'" \
    401 \
    "无效"

# 8. 测试权限不足（普通用户创建用户）
run_test \
    "权限不足" \
    "使用普通用户令牌创建用户，应返回403状态码和权限不足消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $USER_TOKEN' \
    -d '{
        \"name\": \"forbiddentestuser\",
        \"email\": \"forbidden@example.com\",
        \"password\": \"ValidPass123!\"
    }'" \
    403 \
    "权限不足"

# 9. 测试请求格式错误（非JSON格式）
run_test \
    "请求格式错误" \
    "发送非JSON格式的请求体，应返回400状态码和格式错误消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d 'invalid json format'" \
    400 \
    "参数绑定失败"

# 10. 测试邮箱格式不正确
run_test \
    "邮箱格式不正确" \
    "发送格式错误的邮箱地址，应返回422状态码和验证错误消息" \
    "curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL \
    -H 'Content-Type: application/json' \
    -H 'Authorization: Bearer $ADMIN_TOKEN' \
    -d '{
        \"name\": \"bademailuser\",
        \"email\": \"not-an-email\",
        \"password\": \"ValidPass123!\"
    }'" \
    422 \
    "不符合规则"

# 输出测试总结
blue "\n===== 测试总结 ====="
echo "总测试数: $total"
green "通过: $passed"
red "失败: $failed"

if [ $failed -eq 0 ]; then
    green "\n🎉 所有测试通过!"
else
    red "\n❌ 有 $failed 个测试失败，请检查问题。"
fi

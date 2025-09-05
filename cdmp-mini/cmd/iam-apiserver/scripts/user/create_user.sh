#!/bin/bash

# 测试配置
BASE_URL="http://localhost:8080/v1/users"
ADMIN_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzA5NjI5NywiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTcwMDk4OTcsInN1YiI6ImFkbWluIn0.ycLM6HbmMHyQzAqyJNZndCfSGMyhVQELuCX2IpIGM-Y"
USER_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzA5ODMyNywiaWRlbnRpdHkiOiJnZXR0ZXN0LXVzZXIxMDEiLCJpc3MiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsIm9yaWdfaWF0IjoxNzU3MDExOTI3LCJzdWIiOiJnZXR0ZXN0LXVzZXIxMDEifQ.1wyINIb2X6ZHArx_mOFWa0aQ_GCbJu9FpgYRag600fk"
TEMP_DIR="./temp_json"
TEST_USERNAME="testuser$(date +%s)"  # 基础用户名（确保唯一）
TEST_EMAIL="test$(date +%s)@example.com"  # 基础邮箱（确保唯一）

# 颜色输出函数
green() { echo -e "\033[32m$1\033[0m"; }
red() { echo -e "\033[31m$1\033[0m"; }
blue() { echo -e "\033[34m$1\033[0m"; }

# 日志函数
log_info() { echo -e "\033[34m[INFO]\033[0m $1" >&1; }

# 1. 初始化临时目录
init_temp_dir() {
    if [ ! -d "$TEMP_DIR" ]; then
        mkdir -p "$TEMP_DIR"
        log_info "临时目录创建成功: $TEMP_DIR"
    fi
}

# 2. 生成唯一InstanceID（无特殊字符）
generate_instance_id() {
    echo "usr$(date +%s)"
}

# 3. 清理临时文件
cleanup_temp() {
    if [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
        log_info "临时文件已清理"
    fi
}

# 测试结果统计
total=0
passed=0
failed=0

# 核心函数：解析响应消息（兼容转义字符和各种JSON格式）
parse_message() {
    local result="$1"
    # 移除尾部的http_status干扰
    local json_part=$(echo "$result" | sed 's/"http_status":[0-9]*//')
    
    # 优先使用jq解析（最可靠，支持转义字符）
    if command -v jq &> /dev/null; then
        echo "$json_part" | jq -r '.message // .msg // ""'
    else
        # 无jq时使用awk兼容解析
        echo "$json_part" | awk -F'"' '/"message"/ || /"msg"/ {
            for (i=1; i<=NF; i++) {
                if ($i == "message" || $i == "msg") {
                    # 拼接所有字段，补回双引号
                    for (j=i+2; j<=NF; j++) {
                        if (j > i+2) printf "\"";
                        printf "%s", $j;
                    }
                    exit
                }
            }
        }'
    fi
}

# 测试执行函数
run_test() {
    local test_name="$1"
    local description="$2"
    local json_file="$3"
    local auth_token="$4"
    local expected_code="$5"
    local expected_message="$6"

    total=$((total + 1))
    blue "\n=== 测试 $total: $test_name ==="
    echo "描述: $description"
    echo "请求体JSON内容（语法校验后）:"
    
    # JSON语法校验（依赖jq）
    if command -v jq &> /dev/null; then
        if jq . "$json_file" &> /dev/null; then
            green "✅ JSON语法校验通过"
            cat "$json_file"
        else
            red "❌ JSON语法错误！"
            cat "$json_file"
            return
        fi
    else
        echo "（未安装jq，跳过语法校验）"
        cat "$json_file"
    fi

    # 构建curl命令
    local curl_cmd="curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL -H 'Content-Type: application/json' -d @$json_file"
    if [ -n "$auth_token" ]; then
        curl_cmd="$curl_cmd -H 'Authorization: Bearer $auth_token'"
    fi

    echo "执行curl命令: $curl_cmd"
    
    # 执行请求并解析结果
    result=$(eval "$curl_cmd")
    echo "完整响应结果: $result"
    
    # 解析HTTP状态码
    http_code=$(echo "$result" | awk -F'"http_status":' '{print $2}' | grep -o '[0-9]*' | head -n 1)
    if [ -z "$http_code" ]; then
        http_code="0"
    fi

    # 解析响应码
    response_code=$(echo "$result" | awk -F'"code":' '{print $2}' | grep -o '[0-9]*' | head -n 1)

    # 解析响应消息（使用优化后的解析函数）
    message=$(parse_message "$result")

    # 显示解析结果
    echo "实际HTTP状态码: $http_code"
    echo "实际响应码: $response_code"
    echo "实际响应消息: $message"
    echo "预期响应消息包含: $expected_message"
    
    # 验证结果
    if [ "$http_code" -eq "$expected_code" ] && [[ "$message" == *"$expected_message"* ]]; then
        green "✅ 测试通过"
        passed=$((passed + 1))
    else
        red "❌ 测试失败"
        echo "预期HTTP状态码: $expected_code"
        echo "预期响应消息包含: $expected_message"
        failed=$((failed + 1))
    fi

    # 删除当前测试的临时JSON
    rm -f "$json_file"
}

# ==================== 所有10个测试用例执行 ====================
init_temp_dir

# 1. 测试1：使用正确参数创建用户
TEST1_INSTANCE_ID=$(generate_instance_id)
test1_json="$TEMP_DIR/test1.json"
cat > "$test1_json" <<EOF
{
    "metadata": {
        "name": "$TEST_USERNAME",
        "instanceID": "$TEST1_INSTANCE_ID",
        "extend": {}
    },
    "email": "$TEST_EMAIL",
    "password": "ValidPass123!",
    "nickname": "TestUserNickname",
    "phone": "13800138000",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "使用正确参数创建用户" \
    "JSON无注释+含必填nickname，应返回201" \
    "$test1_json" \
    "$ADMIN_TOKEN" \
    201 \
    "用户创建成功"

# 2. 测试2：创建已存在的用户
TEST2_INSTANCE_ID=$(generate_instance_id)
test2_json="$TEMP_DIR/test2.json"
cat > "$test2_json" <<EOF
{
    "metadata": {
        "name": "$TEST_USERNAME",
        "instanceID": "$TEST2_INSTANCE_ID",
        "extend": {}
    },
    "email": "another$TEST_EMAIL",
    "password": "ValidPass123!",
    "nickname": "DuplicateNickname",
    "phone": "13900139000",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "创建已存在的用户" \
    "用户名重复，应返回409" \
    "$test2_json" \
    "$ADMIN_TOKEN" \
    409 \
    "用户已经存在"

# 3. 测试3：缺少必填字段（metadata.name）
test3_json="$TEMP_DIR/test3.json"
cat > "$test3_json" <<EOF
{
    "metadata": {
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "missingusername@example.com",
    "password": "ValidPass123!",
    "nickname": "MissingNameNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "缺少必填字段（metadata.name）" \
    "用户名缺失，应返回422" \
    "$test3_json" \
    "$ADMIN_TOKEN" \
    422 \
    "名称部分不能为空"

# 4. 测试4：用户名不合法（含@）
test4_json="$TEMP_DIR/test4.json"
cat > "$test4_json" <<EOF
{
    "metadata": {
        "name": "invalid@username",
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "invalidusername@example.com",
    "password": "ValidPass123!",
    "nickname": "InvalidNameNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "用户名不合法（含@）" \
    "用户名含特殊字符，应返回422" \
    "$test4_json" \
    "$ADMIN_TOKEN" \
    422 \
    "名称部分必须由字母、数字、'-'、'_'或'.'组成"

# 5. 测试5：密码不符合规则（弱密码123）
test5_json="$TEMP_DIR/test5.json"
cat > "$test5_json" <<EOF
{
    "metadata": {
        "name": "weakpassuser$(date +%s)",
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "weakpass@example.com",
    "password": "123",
    "nickname": "WeakPassNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "密码不符合规则（弱密码123）" \
    "密码不满足规则，应返回422" \
    "$test5_json" \
    "$ADMIN_TOKEN" \
    422 \
    "密码设定不符合规则"

# 6. 测试6：未提供Authorization头
test6_json="$TEMP_DIR/test6.json"
cat > "$test6_json" <<EOF
{
    "metadata": {
        "name": "noauthuser$(date +%s)",
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "noauth@example.com",
    "password": "ValidPass123!",
    "nickname": "NoAuthNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "未提供Authorization头" \
    "无认证令牌，应返回401" \
    "$test6_json" \
    "" \
    401 \
    "缺少 Authorization 头"

# 7. 测试7：使用无效token
test7_json="$TEMP_DIR/test7.json"
cat > "$test7_json" <<EOF
{
    "metadata": {
        "name": "invalidtokenuser$(date +%s)",
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "invalidtoken@example.com",
    "password": "ValidPass123!",
    "nickname": "InvalidTokenNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "使用无效token" \
    "令牌格式错误，应返回401" \
    "$test7_json" \
    "invalid-token" \
    401 \
    "token contains an invalid number of segments"

# 8. 测试8：权限不足（普通用户创建用户）
test8_json="$TEMP_DIR/test8.json"
cat > "$test8_json" <<EOF
{
    "metadata": {
        "name": "forbiddenuser$(date +%s)",
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "forbidden@example.com",
    "password": "ValidPass123!",
    "nickname": "ForbiddenNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "权限不足（普通用户创建用户）" \
    "普通用户无创建权限，应返回403" \
    "$test8_json" \
    "$USER_TOKEN" \
    403 \
    "权限不足"

# 9. 测试9：请求格式错误（非JSON）
blue "\n=== 测试 $((total + 1)): 请求格式错误 ==="
echo "描述: 非JSON请求体，应返回400"
echo "请求体内容: invalid-json-format"

curl_cmd="curl -s -w '\"http_status\":%{http_code}' -X POST $BASE_URL -H 'Content-Type: application/json' -H 'Authorization: Bearer $ADMIN_TOKEN' -d 'invalid-json-format'"
echo "执行curl命令: $curl_cmd"
result=$(eval "$curl_cmd")
echo "完整响应结果: $result"

# 解析结果
total=$((total + 1))
http_code=$(echo "$result" | awk -F'"http_status":' '{print $2}' | grep -o '[0-9]*' | head -n 1)
message=$(parse_message "$result")
if [ -z "$http_code" ]; then
    http_code="0"
fi

echo "实际HTTP状态码: $http_code"
echo "实际响应消息: $message"
if [ "$http_code" -eq 400 ] && [[ "$message" == *"参数绑定失败"* ]]; then
    green "✅ 测试通过"
    passed=$((passed + 1))
else
    red "❌ 测试失败"
    echo "预期HTTP状态码: 400"
    echo "预期响应消息包含: 参数绑定失败"
    failed=$((failed + 1))
fi

# 10. 测试10：邮箱格式不正确（无@）
test10_json="$TEMP_DIR/test10.json"
cat > "$test10_json" <<EOF
{
    "metadata": {
        "name": "bademailuser$(date +%s)",
        "instanceID": "$(generate_instance_id)",
        "extend": {}
    },
    "email": "notanemail",
    "password": "ValidPass123!",
    "nickname": "BadEmailNickname",
    "status": 1,
    "loginedAt": "2024-09-05T12:00:00Z"
}
EOF
run_test \
    "邮箱格式不正确（无@）" \
    "邮箱不合法，应返回422" \
    "$test10_json" \
    "$ADMIN_TOKEN" \
    422 \
    "Email must be a valid email address"

# ==================== 测试总结 ====================
blue "\n===== 测试总结 ====="
echo "总测试数: $total"
green "通过: $passed"
red "失败: $failed"

cleanup_temp

# 结果提示
if [ $failed -gt 0 ]; then
    red "\n❌ 存在失败用例，建议："
    echo "1. 检查测试8：若失败，需修复后端普通用户的权限控制逻辑；"
    echo "2. 其他失败：通过完整响应结果确认消息是否匹配预期；"
    echo "3. 推荐安装jq工具提升解析可靠性：sudo yum install jq 或 sudo apt install jq。"
else
    green "\n🎉 所有测试用例均通过!"
fi
    
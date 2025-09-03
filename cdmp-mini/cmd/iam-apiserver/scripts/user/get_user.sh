#!/bin/bash
# 高并发测试用户查询接口（手动输入令牌，100%可靠）
# 使用方法: 
# 1. 先手动获取令牌
# 2. 执行: ./get_user_manual.sh [令牌] [并发数] [每个线程请求数]

# 配置
IAM_API_URL="http://127.0.0.1:8080"
BASE_USER="gettest-user"
START_USER_ID=1
END_USER_ID=1000

# 参数处理
TOKEN="$1"
CONCURRENCY=${2:-50}
REQUESTS_PER_THREAD=${3:-20}

# 检查令牌是否提供
if [ -z "$TOKEN" ]; then
    echo "用法: $0 [令牌] [并发数] [每个线程请求数]"
    echo "示例: $0 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...' 100 50"
    exit 1
fi

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 统计变量
total_success=0
total_fail=0
total_time=0
declare -A status_codes

# 日志函数
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# 单个线程的查询函数
thread_query() {
    local thread_id=$1
    local token=$2
    local thread_success=0
    local thread_fail=0
    local thread_time=0

    log_info "线程 $thread_id 开始执行，将执行 $REQUESTS_PER_THREAD 次查询"

    for ((i=1; i<=REQUESTS_PER_THREAD; i++)); do
        # 随机选择一个用户
        user_id=$((RANDOM % (END_USER_ID - START_USER_ID + 1) + START_USER_ID))
        username="${BASE_USER}${user_id}"
        url="${IAM_API_URL}/v1/users/${username}"

        # 执行查询并记录时间
        start_time=$(date +%s%3N)
        response=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer ${token}" "$url")
        end_time=$(date +%s%3N)
        duration=$((end_time - start_time))
        thread_time=$((thread_time + duration))

        # 解析响应状态
        http_code=$(echo "$response" | tail -n1)
        body=$(echo "$response" | head -n -1)
        ((status_codes[$http_code]++))

        # 统计结果
        if [ "$http_code" -eq 200 ]; then
            ((thread_success++))
            if ((i % 10 == 0)); then
                log_info "线程 $thread_id: 已完成 $i/$REQUESTS_PER_THREAD 次查询 (成功: $thread_success)"
            fi
        else
            ((thread_fail++))
            if [ "$http_code" -eq 403 ]; then
                log_error "线程 $thread_id: 查询 $username 无权限 (HTTP 403)"
            elif ((thread_fail % 50 != 0)); then
                log_error "线程 $thread_id: 查询 $username 失败 (HTTP $http_code)"
            fi
        fi
    done

    log_info "线程 $thread_id 完成: 成功 $thread_success, 失败 $thread_fail, 总耗时 ${thread_time}ms"
    echo "$thread_success $thread_fail $thread_time" > "thread_${thread_id}.log"
}

# 主逻辑
log_info "=== 用户查询接口高并发测试 ==="
log_info "目标接口: ${IAM_API_URL}/v1/users/:name"
log_info "并发数: $CONCURRENCY, 总请求数: $((CONCURRENCY * REQUESTS_PER_THREAD))"
log_info "使用的令牌（前20位）: ${TOKEN:0:20}..."

# 验证令牌有效性
log_info "验证令牌有效性..."
test_response=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer ${TOKEN}" "${IAM_API_URL}/v1/users/${BASE_USER}1")
test_code=$(echo "$test_response" | tail -n1)

if [ "$test_code" -eq 403 ]; then
    log_error "令牌无查询权限（HTTP 403）"
    exit 1
elif [ "$test_code" -eq 404 ]; then
    log_warn "测试用户 ${BASE_USER}1 不存在，可能影响结果"
elif [ "$test_code" -ne 200 ]; then
    log_error "令牌验证失败（HTTP $test_code）"
    exit 1
else
    log_success "令牌验证成功，权限正常"
fi

# 开始高并发测试
test_start_time=$(date +%s%3N)
log_info "开始启动 $CONCURRENCY 个线程..."

for ((i=1; i<=CONCURRENCY; i++)); do
    thread_query $i "$TOKEN" &
done

# 等待所有线程完成
wait

# 汇总结果
for ((i=1; i<=CONCURRENCY; i++)); do
    if [ -f "thread_${i}.log" ]; then
        read success fail time < "thread_${i}.log"
        total_success=$((total_success + success))
        total_fail=$((total_fail + fail))
        total_time=$((total_time + time))
        rm "thread_${i}.log"
    fi
done

# 输出最终统计
test_end_time=$(date +%s%3N)
test_duration=$((test_end_time - test_start_time))

log_info "\n=== 测试结果汇总 ==="
log_success "总请求数: $((CONCURRENCY * REQUESTS_PER_THREAD))"
log_success "成功数: $total_success (${GREEN}$((total_success * 100 / (total_success + total_fail)))%${NC})"
log_error "失败数: $total_fail (${RED}$((total_fail * 100 / (total_success + total_fail)))%${NC})"
log_info "平均响应时间: $((total_time / (total_success + total_fail)))ms"
log_info "总耗时: ${test_duration}ms"
log_info "状态码分布:"
for code in "${!status_codes[@]}"; do
    log_info "  $code: ${status_codes[$code]}次"
done

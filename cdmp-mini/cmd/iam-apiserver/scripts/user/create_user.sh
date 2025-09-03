#!/bin/bash
# create_users.sh - 用户创建脚本
# 功能：支持单次创建一个用户、批量创建两个用户（无重复检查）
# 使用方法:
#   单次创建一个用户: ./create_users.sh batch <用户名> <邮箱> <令牌>
#   批量创建2个用户: ./create_users.sh batch-2 <令牌>
#   批量创建1000个用户: ./create_users.sh batch-1000 <令牌>

# 配置
IAM_API_URL="http://127.0.0.1:8080"
BATCH_SIZE=50  # 每批创建数量（用于1000用户场景）
DELAY=0.5      # 批次间延迟（秒）

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 日志函数
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1" >&1; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&1; }
log_info() { echo -e "${BLUE}[INFO]${NC} $1" >&1; }

# 检查依赖工具
check_dependencies() {
    local deps=("curl" "date")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log_error "缺少必要工具: $dep，请先安装"
            exit 1
        fi
    done
}

# 创建单个用户（无重复检查，由数据库保证唯一性）
create_single_user() {
    local username="$1"
    local email="$2"
    
    local response=$(curl -s -w "\n%{http_code}" -X POST "${IAM_API_URL}/v1/users" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${TOKEN}" \
        -d '{
            "metadata": {
                "name": "'"${username}"'"
            },
            "nickname": "'"${username}"'",
            "email": "'"${email}"'",
            "password": "TestPass123!",
            "status": 1,
            "isAdmin": 0
        }')

    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | head -n -1)
    
    if [ "$http_code" -eq 200 ] || [ "$http_code" -eq 201 ]; then
        log_success "用户 '$username' 创建成功"
        return 0  # 成功
    else
        log_error "用户 '$username' 创建失败 (HTTP $http_code)，响应: $body"
        return 1  # 失败
    fi
}

# 批量创建2个用户（默认命名规则）
batch_create_2() {
    log_info "开始批量创建2个用户..."
    local base_username="test-user"
    local base_email="test-user"
    local domain="example.com"
    local total=2
    local success_count=0
    local fail_count=0
    local start_time=$(date +%s)

    for i in $(seq 1 $total); do
        local username="${base_username}${i}"
        local email="${base_email}${i}@${domain}"
        
        if create_single_user "$username" "$email"; then
            success_count=$((success_count + 1))
        else
            fail_count=$((fail_count + 1))
        fi
    done

    local end_time=$(date +%s)
    local duration=$(( end_time - start_time ))
    
    log_info "======================================"
    log_success "批量创建完成 - 总耗时: ${duration}秒"
    log_success "成功: $success_count, 失败: $fail_count"
}

# 批量创建1000个用户（默认命名规则）
batch_create_1000() {
    log_info "开始批量创建1000个用户..."
    local base_username="gettest-user"
    local base_email="gettest-user"
    local domain="example.com"
    local total=1000
    local success_count=0
    local fail_count=0
    local start_time=$(date +%s)

    # 分批次创建
    for batch in $(seq 0 $(( (total - 1) / BATCH_SIZE )) ); do
        local start=$(( batch * BATCH_SIZE + 1 ))
        local end=$(( (batch + 1) * BATCH_SIZE ))
        if [ $end -gt $total ]; then
            end=$total
        fi

        log_info "处理批次 $((batch + 1)): 创建用户 $start-$end"
        
        # 本批次内的用户创建
        for i in $(seq $start $end); do
            local username="${base_username}${i}"
            local email="${base_email}${i}@${domain}"
            
            if create_single_user "$username" "$email"; then
                success_count=$((success_count + 1))
                # 每成功20个输出一次进度
                if (( success_count % 20 == 0 )); then
                    log_info "已成功创建 $success_count 个用户"
                fi
            else
                fail_count=$((fail_count + 1))
            fi
        done

        # 批次间延迟（最后一批不延迟）
        if [ $end -lt $total ]; then
            log_info "批次间延迟 ${DELAY}秒..."
            sleep $DELAY
        fi
    done

    local end_time=$(date +%s)
    local duration=$(( end_time - start_time ))
    
    log_info "======================================"
    log_success "批量创建完成 - 总耗时: ${duration}秒"
    log_success "成功: $success_count, 失败: $fail_count"
}

# 单次创建一个用户（通过batch命令）
batch_create_one() {
    local username="$1"
    local email="$2"
    
    log_info "开始创建单个用户: $username"
    local start_time=$(date +%s)
    
    if create_single_user "$username" "$email"; then
        log_success "单个用户创建成功"
    else
        log_error "单个用户创建失败"
    fi

    local end_time=$(date +%s)
    local duration=$(( end_time - start_time ))
    log_info "耗时: ${duration}秒"
}

# 显示用法
usage() {
    echo "用法:" >&1
    echo "  单次创建一个用户: $0 batch <用户名> <邮箱> <令牌>" >&1
    echo "  批量创建2个用户: $0 batch-2 <令牌>" >&1
    echo "  批量创建1000个用户: $0 batch-1000 <令牌>" >&1
    echo "  示例: $0 batch myuser myuser@example.com <令牌>" >&1
}

# 主逻辑入口
log_info "=== 用户创建脚本启动 ==="

# 检查依赖
check_dependencies

# 验证参数
if [ $# -lt 2 ]; then
    log_error "参数不足"
    usage
    exit 1
fi

# 解析命令和令牌
COMMAND="$1"

# 根据不同命令处理参数
case "$COMMAND" in
    batch-2)
        if [ $# -ne 2 ]; then
            log_error "参数错误，正确用法: $0 batch-2 <令牌>"
            exit 1
        fi
        TOKEN="$2"
        batch_create_2
        ;;
    batch-1000)
        if [ $# -ne 2 ]; then
            log_error "参数错误，正确用法: $0 batch-1000 <令牌>"
            exit 1
        fi
        TOKEN="$2"
        batch_create_1000
        ;;
    batch)
        if [ $# -ne 4 ]; then
            log_error "参数错误，正确用法: $0 batch <用户名> <邮箱> <令牌>"
            exit 1
        fi
        local username="$2"
        local email="$3"
        TOKEN="$4"
        batch_create_one "$username" "$email"
        ;;
    *)
        log_error "未知命令: $1"
        usage
        exit 1
        ;;
esac

log_info "=== 脚本执行完成 ==="

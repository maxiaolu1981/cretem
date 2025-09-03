#!/bin/bash
# 单条用户硬删除操作脚本（适配控制层ForceDelete接口）
# 功能：调用DELETE /v1/users/:name/force接口硬删除指定用户
# 使用方法: ./delete_user_hard.sh [令牌] [待删除用户名]
# 示例: ./delete_user_hard.sh "eyJhbGciOiJIUzI1Ni..." "test-user123"

# 配置
API_BASE_URL="http://127.0.0.1:8080/v1/users"  # 需与后端接口地址一致
TIMEOUT=10  # 超时时间(秒)

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 日志函数
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# 参数校验
if [ $# -ne 2 ]; then
    log_error "参数错误！正确用法:"
    log_error "  ./delete_user_hard.sh [令牌] [待删除用户名]"
    exit 1
fi

TOKEN="$1"
DELETE_USER="$2"
# 关键修改：添加/force路径后缀，适配硬删除接口
DELETE_URL="${API_BASE_URL}/${DELETE_USER}/force"

# 检查令牌和用户名不为空
if [ -z "$TOKEN" ]; then
    log_error "令牌不能为空，请提供有效的访问令牌"
    exit 1
fi

if [ -z "$DELETE_USER" ]; then
    log_error "待删除的用户名不能为空"
    exit 1
fi

# 硬删除风险提示与确认
log_warn "警告：这是硬删除操作，数据将被永久删除且无法恢复！"
read -p "确认要硬删除用户 '$DELETE_USER' 吗？(y/N) " CONFIRM
if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
    log_info "操作已取消"
    exit 0
fi

# 执行删除请求
log_info "开始硬删除用户: $DELETE_USER"
log_info "请求接口: $DELETE_URL"

# 发送DELETE请求（携带Bearer令牌）
response=$(curl -s -w "\n%{http_code}" -X DELETE "${DELETE_URL}" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    --max-time ${TIMEOUT})

# 解析响应（分离响应体和HTTP状态码）
http_code=$(echo "$response" | tail -n1)
response_body=$(echo "$response" | head -n -1)

# 处理结果（根据控制层逻辑适配）
case $http_code in
    200)
        # 后端成功硬删除时返回200
        log_success "用户 '$DELETE_USER' 已永久删除"
        log_info "响应: $response_body"
        exit 0
        ;;
    401)
        # 控制层：未登录时返回401
        log_error "删除失败：未授权（令牌无效或已过期）"
        log_error "响应: $response_body"
        exit 1
        ;;
    404)
        # 用户不存在
        log_warn "用户 '$DELETE_USER' 不存在（无需删除）"
        log_info "响应: $response_body"
        exit 0
        ;;
    403)
        # 权限不足，无硬删除权限
        log_error "删除失败：权限不足（没有硬删除操作权限）"
        log_error "响应: $response_body"
        exit 1
        ;;
    400)
        # 参数错误
        log_error "删除失败：请求参数错误"
        log_error "响应: $response_body"
        exit 1
        ;;
    500)
        # 服务器内部错误
        log_error "删除失败：服务器内部错误"
        log_error "响应: $response_body"
        exit 1
        ;;
    *)
        log_error "删除失败：未知错误（HTTP状态码: $http_code）"
        log_error "响应: $response_body"
        exit 1
        ;;
esac

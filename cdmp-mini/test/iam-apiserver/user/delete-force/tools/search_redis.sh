#!/bin/bash

# 默认参数
KEY_PATTERN=""
SHOW_VALUE=false
CLEAR_DATA=false
SHOW_ALL=false

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -k|--key)
            KEY_PATTERN="$2"
            shift 2
            ;;
        -v|--value)
            SHOW_VALUE=true
            shift
            ;;
        -c|--clear)
            CLEAR_DATA=true
            shift
            ;;
        -a|--all)
            SHOW_ALL=true
            shift
            ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo "选项:"
            echo "  -k, --key PATTERN    指定要查找的key模式"
            echo "  -v, --value          显示匹配key的值"
            echo "  -c, --clear          在执行查询前清空所有Redis数据"
            echo "  -a, --all            显示所有key和value"
            echo "  -h, --help           显示帮助信息"
            exit 0
            ;;
        *)
            echo "未知参数: $1"
            exit 1
            ;;
    esac
done

# 清空Redis数据（如果指定了-c参数）
if [ "$CLEAR_DATA" = true ]; then
    echo "=== 清空 Redis 数据 ==="
    echo "警告：此操作将永久删除所有Redis数据！"
    read -p "确认要清空所有Redis数据吗？(y/N): " confirm
    if [[ $confirm =~ ^[Yy]$ ]]; then
        for port in 6379 6380 6381; do
            echo "清空端口 $port 的数据..."
            redis-cli -h 192.168.10.14 -p $port FLUSHALL
            if [ $? -eq 0 ]; then
                echo "端口 $port 数据清空成功"
            else
                echo "端口 $port 数据清空失败"
            fi
        done
        echo ""
    else
        echo "已取消清空操作"
        echo ""
    fi
fi

echo "=== Redis 集群状态检查 ==="

# 显示所有key和value（如果指定了-a参数）
if [ "$SHOW_ALL" = true ]; then
    echo "=== 所有 Key 和 Value ==="
    for port in 6379 6380 6381; do
        echo "端口 $port 的所有数据:"
        redis-cli -h 192.168.10.14 -p $port --scan --pattern "*" | while read key; do
            value=$(redis-cli -h 192.168.10.14 -p $port GET "$key" 2>/dev/null)
            # 处理可能的多行value
            value_single_line=$(echo "$value" | tr '\n' ' ' | sed 's/[[:space:]]*$//')
            echo "  $key = $value_single_line"
        done
        echo "---"
    done
    echo ""
fi

# 统计 genericapiserver:test_ 相关的key
echo "=== genericapiserver:test_ Key 统计 ==="
TOTAL_USER_KEYS=0
for port in 6379 6380 6381; do
    USER_KEY_COUNT=$(redis-cli -h 192.168.10.14 -p $port --scan --pattern "*genericapiserver:test_*" | wc -l)
    echo "端口 $port: $USER_KEY_COUNT 个 genericapiserver:test_ key"
    TOTAL_USER_KEYS=$((TOTAL_USER_KEYS + USER_KEY_COUNT))
done
echo "总计: $TOTAL_USER_KEYS 个 genericapiserver:test_ key"
echo ""

# 统计 value 为 RATE_LIMIT_PREVENTION 的key数量
echo "=== RATE_LIMIT_PREVENTION Value 统计 ==="
TOTAL_RATE_LIMIT_KEYS=0
for port in 6379 6380 6381; do
    RATE_LIMIT_COUNT=0
    # 扫描所有key并检查value
    keys=$(redis-cli -h 192.168.10.14 -p $port --scan --pattern "*")
    if [ -n "$keys" ]; then
        while read key; do
            [ -z "$key" ] && continue
            value=$(redis-cli -h 192.168.10.14 -p $port GET "$key" 2>/dev/null)
            if [ "$value" = "RATE_LIMIT_PREVENTION" ]; then
                RATE_LIMIT_COUNT=$((RATE_LIMIT_COUNT + 1))
            fi
        done <<< "$keys"
    fi
    echo "端口 $port: $RATE_LIMIT_COUNT 个 value=RATE_LIMIT_PREVENTION 的key"
    TOTAL_RATE_LIMIT_KEYS=$((TOTAL_RATE_LIMIT_KEYS + RATE_LIMIT_COUNT))
done
echo "总计: $TOTAL_RATE_LIMIT_KEYS 个 value=RATE_LIMIT_PREVENTION 的key"
echo ""

# 如果指定了key模式，搜索并显示
if [ -n "$KEY_PATTERN" ]; then
    echo "=== 搜索 Key 模式: $KEY_PATTERN ==="
    for port in 6379 6380 6381; do
        echo "端口 $port 的匹配结果:"
        redis-cli -h 192.168.10.14 -p $port --scan --pattern "*$KEY_PATTERN*" | while read key; do
            if [ "$SHOW_VALUE" = true ] || [ "$SHOW_ALL" = true ]; then
                value=$(redis-cli -h 192.168.10.14 -p $port GET "$key" 2>/dev/null)
                # 处理可能的多行value
                value_single_line=$(echo "$value" | tr '\n' ' ' | sed 's/[[:space:]]*$//')
                echo "  $key = $value_single_line"
            else
                echo "  $key"
            fi
        done
    done
    echo ""
fi

# 显示基本集群信息
echo "=== 集群基本信息 ==="
for port in 6379 6380 6381; do
    echo "端口 $port:"
    echo " - Key数量: $(redis-cli -h 192.168.10.14 -p $port DBSIZE)"
    echo " - 内存使用: $(redis-cli -h 192.168.10.14 -p $port INFO memory | grep used_memory_human | cut -d: -f2)"
    cluster_info=$(redis-cli -h 192.168.10.14 -p $port CLUSTER NODES 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo " - 连接地址: $(echo "$cluster_info" | grep myself | awk '{print $2}')"
    else
        echo " - 连接地址: 单节点模式"
    fi
    echo "---"
done
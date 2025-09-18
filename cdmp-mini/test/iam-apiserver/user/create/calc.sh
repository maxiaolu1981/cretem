#!/bin/bash

# 计算Kafka中指定主题的未被消费记录数量
# 支持多个主题，默认包含user.create.v1
# 使用方法: ./calculate_unconsumed_multitopic.sh <kafka-bootstrap-servers> [topic1] [topic2] ...
# 示例: ./calculate_unconsumed_multitopic.sh localhost:9092 user.create.v1 user.update.v1

# 检查参数
if [ $# -lt 1 ]; then
    echo "使用方法: $0 <kafka-bootstrap-servers> [topic1] [topic2] ..."
    echo "示例: $0 localhost:9092 user.create.v1 user.update.v1"
    echo "默认主题: 若不指定主题，将检查 user.create.v1"
    exit 1
fi

BOOTSTRAP_SERVERS=$1
shift  # 移除第一个参数（bootstrap服务器）

# 设置要检查的主题，默认包含user.create.v1
if [ $# -eq 0 ]; then
    TOPICS=("user.create.v1")
else
    TOPICS=("$@")
fi

KAFKA_CONSUMER_GROUPS="kafka-consumer-groups.sh"

# 检查kafka-consumer-groups.sh是否可用
if ! command -v $KAFKA_CONSUMER_GROUPS &> /dev/null; then
    # 尝试常见的Kafka安装路径
    if [ -f "/opt/kafka/bin/$KAFKA_CONSUMER_GROUPS" ]; then
        KAFKA_CONSUMER_GROUPS="/opt/kafka/bin/$KAFKA_CONSUMER_GROUPS"
    elif [ -f "/usr/local/kafka/bin/$KAFKA_CONSUMER_GROUPS" ]; then
        KAFKA_CONSUMER_GROUPS="/usr/local/kafka/bin/$KAFKA_CONSUMER_GROUPS"
    else
        echo "错误: 找不到kafka-consumer-groups.sh，请确保Kafka命令行工具已安装并在PATH中"
        exit 1
    fi
fi

echo "=== 开始计算Kafka主题未消费记录 ==="
echo "Kafka bootstrap服务器: $BOOTSTRAP_SERVERS"
echo "要检查的主题: ${TOPICS[*]}"
echo "----------------------------------------"

# 初始化所有主题的总未消费消息数
GRAND_TOTAL=0

# 遍历每个主题
for TOPIC in "${TOPICS[@]}"; do
    echo "========================================"
    echo "处理主题: $TOPIC"
    echo "----------------------------------------"
    
    # 初始化当前主题的未消费消息数
    TOTAL_UNCONSUMED=0
    
    # 获取消费该主题的所有消费者组
    echo "正在获取消费 $TOPIC 的消费者组..."
    CONSUMER_GROUPS=$($KAFKA_CONSUMER_GROUPS --bootstrap-server $BOOTSTRAP_SERVERS --list --topic $TOPIC 2>/dev/null)
    
    if [ -z "$CONSUMER_GROUPS" ]; then
        echo "警告: 没有找到消费 $TOPIC 的消费者组"
        
        # 计算主题总消息数（此时所有消息均未被消费）
        echo "正在计算主题 $TOPIC 的总消息数..."
        # 使用GetOffsetShell获取每个分区的最新偏移量
        if command -v kafka-run-class.sh &> /dev/null; then
            OFFSET_DETAILS=$(kafka-run-class.sh kafka.tools.GetOffsetShell --bootstrap-server $BOOTSTRAP_SERVERS --topic $TOPIC --time -1 2>/dev/null)
            TOTAL_MESSAGES=$(echo "$OFFSET_DETAILS" | awk -F: '{sum += $3} END {print sum}')
        else
            # 备选方法：从describe命令获取
            TOTAL_MESSAGES=$($KAFKA_CONSUMER_GROUPS --bootstrap-server $BOOTSTRAP_SERVERS --describe --topic $TOPIC --all-groups 2>/dev/null | grep -v "GROUP" | awk '{sum += $5} END {print sum}')
        fi
        
        if [ -z "$TOTAL_MESSAGES" ] || [ "$TOTAL_MESSAGES" -eq 0 ]; then
            echo "无法获取主题 $TOPIC 的消息数量或主题为空"
            TOTAL_UNCONSUMED=0
        else
            echo "主题 $TOPIC 总消息数（全部未被消费）: $TOTAL_MESSAGES"
            TOTAL_UNCONSUMED=$TOTAL_MESSAGES
        fi
    else
        # 遍历每个消费者组
        while IFS= read -r GROUP; do
            echo "----------------------------------------"
            echo "处理消费者组: $GROUP"
            
            # 获取该消费者组的详细消费信息
            GROUP_DETAILS=$($KAFKA_CONSUMER_GROUPS --bootstrap-server $BOOTSTRAP_SERVERS --describe --group $GROUP --topic $TOPIC 2>/dev/null)
            
            # 显示该消费者组的消费情况
            echo "分区消费情况:"
            echo "分区 | 已消费偏移量 | 最新偏移量 | 未消费数量"
            echo "----------------------------------------"
            
            # 解析并计算未消费消息数
            GROUP_UNCONSUMED=$(echo "$GROUP_DETAILS" | grep -v "GROUP" | awk '{
                lag = $5 - $4;
                if (lag < 0) lag = 0;  # 防止出现负数（可能由于消费者组重置偏移量）
                printf "%-4s | %-12s | %-10s | %-12s\n", $3, $4, $5, lag;
                sum += lag
            } END {print sum}')
            
            # 提取该消费者组的未消费总数
            GROUP_TOTAL=$(echo "$GROUP_UNCONSUMED" | tail -n 1)
            # 显示分区详情（排除最后一行的总和）
            echo "$GROUP_UNCONSUMED" | head -n -1
            
            echo "----------------------------------------"
            echo "消费者组 $GROUP 未消费消息总数: $GROUP_TOTAL"
            
            # 累加至当前主题的未消费消息数
            TOTAL_UNCONSUMED=$((TOTAL_UNCONSUMED + GROUP_TOTAL))
        done <<< "$CONSUMER_GROUPS"
    fi
    
    echo "----------------------------------------"
    echo "主题 $TOPIC 未消费消息总数: $TOTAL_UNCONSUMED"
    GRAND_TOTAL=$((GRAND_TOTAL + TOTAL_UNCONSUMED))
done

echo "========================================"
echo "所有主题的总未消费消息数: $GRAND_TOTAL"
echo "========================================"

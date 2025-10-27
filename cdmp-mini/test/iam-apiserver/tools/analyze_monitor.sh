#!/bin/bash

# 数据库监控数据分析脚本
LOG_FILE="$1"

if [ -z "$LOG_FILE" ] || [ ! -f "$LOG_FILE" ]; then
    echo "用法: $0 <监控文件.csv>"
    exit 1
fi

echo "=== 数据库集群监控分析报告 ==="
echo "分析文件: $LOG_FILE"
echo ""

# 基本信息
FIRST_LINE=$(tail -n +2 "$LOG_FILE" | head -1)
LAST_LINE=$(tail -1 "$LOG_FILE")
FIRST_TIME=$(echo "$FIRST_LINE" | cut -d',' -f1)
LAST_TIME=$(echo "$LAST_LINE" | cut -d',' -f1)
SAMPLE_COUNT=$(tail -n +2 "$LOG_FILE" | wc -l)

echo "监控时间范围: $FIRST_TIME 到 $LAST_TIME"
echo "总采样点数: $SAMPLE_COUNT"
echo ""

# 获取所有节点
NODES=$(tail -n +2 "$LOG_FILE" | cut -d',' -f2 | sort -u)
echo "监控节点: $NODES"
echo ""

# 分析每个节点
for NODE in $NODES; do
    echo "--- 节点 $NODE 分析 ---"
    
    # 提取该节点的数据
    grep ",$NODE," "$LOG_FILE" > "/tmp/node_${NODE}.csv"
    
    # 计算平均值
    AVG_CONNECTIONS=$(awk -F, '{sum+=$3} END {printf "%.2f", sum/NR}' "/tmp/node_${NODE}.csv")
    MAX_CONNECTIONS=$(awk -F, 'max<$3 {max=$3} END {print max}' "/tmp/node_${NODE}.csv")
    AVG_QPS=$(awk -F, '{sum+=$5} END {printf "%.2f", sum/NR}' "/tmp/node_${NODE}.csv")
    MAX_QPS=$(awk -F, 'max<$5 {max=$5} END {print max}' "/tmp/node_${NODE}.csv")
    AVG_HIT_RATE=$(awk -F, '{sum+=$11} END {printf "%.2f", sum/NR}' "/tmp/node_${NODE}.csv")
    TOTAL_LOCK_WAITS=$(awk -F, '{sum+=$12} END {print sum}' "/tmp/node_${NODE}.csv")
    AVG_INSERTS=$(awk -F, '{sum+=$8} END {printf "%.2f", sum/NR}' "/tmp/node_${NODE}.csv")
    TOTAL_INSERTS=$(awk -F, '{sum+=$8} END {print sum}' "/tmp/node_${NODE}.csv")
    
    echo "连接数: 平均=${AVG_CONNECTIONS}, 最大=${MAX_CONNECTIONS}"
    echo "QPS: 平均=${AVG_QPS}, 峰值=${MAX_QPS}"
    echo "缓冲池命中率: ${AVG_HIT_RATE}%"
    echo "行锁等待总次数: ${TOTAL_LOCK_WAITS}"
    echo "插入操作: 平均=${AVG_INSERTS}行/秒, 总计=${TOTAL_INSERTS}行"
    
    # 检测高负载时段
    HIGH_LOAD_COUNT=$(awk -F, '$3 > '"$AVG_CONNECTIONS"' * 1.5 {count++} END {print count}' "/tmp/node_${NODE}.csv")
    if [ "$HIGH_LOAD_COUNT" -gt 0 ]; then
        echo "⚠️  高负载时段: ${HIGH_LOAD_COUNT} 个采样点（>平均1.5倍）"
    fi
    
    # 检测低命中率
    LOW_HIT_COUNT=$(awk -F, '$11 < 95 {count++} END {print count}' "/tmp/node_${NODE}.csv")
    if [ "$LOW_HIT_COUNT" -gt 0 ]; then
        echo "⚠️  低命中率时段: ${LOW_HIT_COUNT} 个采样点（<95%）"
    fi
    
    echo ""
done

# 集群总体分析
echo "=== 集群总体分析 ==="

# 计算集群总QPS
TOTAL_AVG_QPS=$(tail -n +2 "$LOG_FILE" | awk -F, '{qps_sum[$2]+=$5; count[$2]++} END {for(node in qps_sum) total+=qps_sum[node]/count[node]; print total}')
echo "集群平均总QPS: ${TOTAL_AVG_QPS}"

# 找出性能最差的节点
echo ""
echo "性能瓶颈分析:"
for NODE in $NODES; do
    AVG_LOCK_WAITS=$(awk -F, '{sum+=$12} END {printf "%.2f", sum/NR}' "/tmp/node_${NODE}.csv")
    echo "节点 $NODE - 平均锁等待: ${AVG_LOCK_WAITS}"
done | sort -k4 -nr

# 生成简单的ASCII图表
echo ""
echo "=== 连接数趋势 (简易图表) ==="
for NODE in $NODES; do
    echo "节点 $NODE:"
    # 取最后20个采样点的连接数
    tail -20 "/tmp/node_${NODE}.csv" | awk -F, '
    {
        conn = $3;
        max = 50;  # 图表最大宽度
        scaled = int(conn * max / 100);  # 假设最大100连接数
        printf "%-10s |", $1
        for(i=0; i<scaled; i++) printf "█";
        for(i=scaled; i<max; i++) printf " ";
        printf "| %d\n", conn;
    }'
    echo ""
done

# 清理临时文件
for NODE in $NODES; do
    rm -f "/tmp/node_${NODE}.csv"
done

echo "分析完成!"
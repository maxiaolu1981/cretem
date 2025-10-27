#!/bin/bash

# 数据库集群监控配置
DB_USER="root"
DB_PASSWORD="iam59!z$"
DB_BASE="iam"

# 集群节点配置
REPLICA_HOSTS=("192.168.10.8" "192.168.10.8" "192.168.10.8")
REPLICA_PORTS=("3306" "3307" "3308")
NODE_COUNT=3

LOG_FILE="db_cluster_monitor_$(date +%Y%m%d_%H%M%S).csv"
DURATION=60  # 监控时长（秒）
INTERVAL=1   # 采集间隔（秒）

# 创建日志文件头
echo "Timestamp,Node,Threads_connected,Threads_running,Questions,Queries_per_sec,Connections,Innodb_rows_read,Innodb_rows_inserted,Innodb_rows_updated,Innodb_buffer_pool_hit_rate,Innodb_row_lock_waits,Innodb_row_lock_time_avg,Table_locks_waited,Slow_queries,Connection_usage_percent,Replica_lag,Bytes_received,Bytes_sent" > $LOG_FILE

echo "开始监控数据库集群，节点数: $NODE_COUNT，持续时间: $DURATION 秒"
echo "日志文件: $LOG_FILE"

for ((i=1; i<=$DURATION; i++))
do
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # 监控每个节点
    for ((node=0; node<NODE_COUNT; node++))
    do
        HOST=${REPLICA_HOSTS[$node]}
        PORT=${REPLICA_PORTS[$node]}
        
        # 获取数据库状态指标
        METRICS=$(mysql -h$HOST -P$PORT -u$DB_USER -p$DB_PASSWORD -D$DB_BASE -B -N -e "
        SET @max_connections = (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_VARIABLES WHERE VARIABLE_NAME = 'max_connections');
        SET @connections = (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Threads_connected');
        SET @connection_usage = ROUND((@connections / @max_connections) * 100, 2);
        
        SELECT 
            CONCAT(
                IFNULL(Threads_connected, 0), ',',
                IFNULL(Threads_running, 0), ',', 
                IFNULL(Questions, 0), ',',
                IFNULL(Queries, 0), ',',
                IFNULL(Connections, 0), ',',
                IFNULL(Innodb_rows_read, 0), ',',
                IFNULL(Innodb_rows_inserted, 0), ',', 
                IFNULL(Innodb_rows_updated, 0), ',',
                IFNULL(Buffer_pool_hit_rate, 0), ',',
                IFNULL(Innodb_row_lock_waits, 0), ',',
                IFNULL(Innodb_row_lock_time_avg, 0), ',',
                IFNULL(Table_locks_waited, 0), ',',
                IFNULL(Slow_queries, 0), ',',
                IFNULL(@connection_usage, 0), ',',
                IFNULL(Replica_lag, 0), ',',
                IFNULL(Bytes_received, 0), ',',
                IFNULL(Bytes_sent, 0)
            )
        FROM (
            SELECT 
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Threads_connected') as Threads_connected,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Threads_running') as Threads_running,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Questions') as Questions,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Queries') as Queries,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Connections') as Connections,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_rows_read') as Innodb_rows_read,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_rows_inserted') as Innodb_rows_inserted,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_rows_updated') as Innodb_rows_updated,
                ROUND((1 - (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_buffer_pool_reads') / 
                      NULLIF((SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_buffer_pool_read_requests'), 0)) * 100, 2) as Buffer_pool_hit_rate,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_row_lock_waits') as Innodb_row_lock_waits,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Innodb_row_lock_time_avg') as Innodb_row_lock_time_avg,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Table_locks_waited') as Table_locks_waited,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Slow_queries') as Slow_queries,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Seconds_Behind_Master') as Replica_lag,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Bytes_received') as Bytes_received,
                (SELECT VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME = 'Bytes_sent') as Bytes_sent
        ) as metrics;
        " 2>/dev/null)
        
        if [ $? -eq 0 ] && [ ! -z "$METRICS" ]; then
            echo "$TIMESTAMP,node${node}_${PORT},$METRICS" >> $LOG_FILE
        else
            echo "$TIMESTAMP,node${node}_${PORT},ERROR: Connection failed" >> $LOG_FILE
        fi
    done
    
    # 显示进度
    if [ $((i % 10)) -eq 0 ]; then
        echo "已监控: $i 秒"
    fi
    
    sleep $INTERVAL
done

echo "监控完成！数据已保存到: $LOG_FILE"
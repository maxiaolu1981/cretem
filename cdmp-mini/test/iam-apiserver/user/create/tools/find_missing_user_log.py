#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
查找缺失用户名在 consumer/producer 关键日志中的所有相关行，便于定位丢失原因。
用法：
  python3 find_missing_user_log.py missing_usernames.txt app.log > missing_user_log.txt

missing_usernames.txt: 缺失用户名，每行一个
app.log: 业务日志文件

关键字自动提取自 consumer.go/producer.go 日志输出：
- Fetcher
- Worker
- 偏移量提交
- 用户创建/删除/更新
- 缓存
- 死信
- 重试
- 发送成功/失败
- Producer
- in-flight
- 限速
- 失败/成功/警告/Error/Debug
"""
import sys
import re

# 关键字集合（自动提取自 consumer.go/producer.go）
KEYWORDS = [
    'Fetcher', 'Worker', '偏移量', '用户', 'username', '缓存', '死信', '重试',
    '发送成功', '发送失败', 'Producer', 'in-flight', '限速', '失败', '成功', '警告', 'Error', 'Debug',
    '创建', '删除', '更新', 'sendToRetry', 'sendToDeadLetter', 'sendToRetryTopic', 'SendToDeadLetterTopic'
]

if len(sys.argv) != 3:
    print("用法: python3 find_missing_user_log.py missing_usernames.txt app.log")
    sys.exit(1)

with open(sys.argv[1], 'r', encoding='utf-8') as f:
    missing = set(line.strip() for line in f if line.strip())

with open(sys.argv[2], 'r', encoding='utf-8') as f:
    for line in f:
        # 只要包含用户名和关键字之一就输出
        for name in missing:
            if name in line:
                for kw in KEYWORDS:
                    if kw in line:
                        print(line.strip())
                        break
                break

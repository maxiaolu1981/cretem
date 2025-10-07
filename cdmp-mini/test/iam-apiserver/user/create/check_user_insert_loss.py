#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
比对测试用例记录的所有成功 username 与数据库 user 表实际存在的 username，输出丢失的用户名和统计信息。
用法：
  python3 check_user_insert_loss.py success_usernames.txt db_usernames.txt

success_usernames.txt: 测试用例记录的所有成功 username，每行一个
  (如测试用例结束后保存 successUsernames 到该文件)
db_usernames.txt: 数据库导出的所有 user 表 username，每行一个
  (如: select name from user;)
"""
import sys

if len(sys.argv) != 3:
    print("用法: python3 check_user_insert_loss.py success_usernames.txt db_usernames.txt")
    sys.exit(1)

with open(sys.argv[1], 'r', encoding='utf-8') as f:
    success = set(line.strip() for line in f if line.strip())
with open(sys.argv[2], 'r', encoding='utf-8') as f:
    db = set(line.strip() for line in f if line.strip())

lost = success - db
extra = db - success

print(f"测试用例记录成功 username 总数: {len(success)}")
print(f"数据库实际 username 总数: {len(db)}")
print(f"数据库缺失 username 数: {len(lost)}")
if lost:
    # 输出前20行到屏幕
    print("缺失用户名单(前20):")
    for i, name in enumerate(list(lost)[:20]):
        print(f"  {i+1}. {name}")
    # 全量写入 missing_usernames.txt
    with open("missing_usernames.txt", "w", encoding="utf-8") as fout:
        for name in lost:
            fout.write(name + "\n")
    print(f"已将全部缺失用户名写入 missing_usernames.txt，共 {len(lost)} 条")
else:
    print("数据库未缺失任何测试用例成功 username")

if extra:
    print(f"数据库多余 username 数: {len(extra)} (非本次测试产生)")

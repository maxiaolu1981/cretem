 watch -n 2 'echo -n "总行数: "; wc -l 2025-10-18.json | awk "{print \$1}"; echo -n "文件大小: "; du -h 2025-10-18.json | awk "{print \$1}"'
----持续监测文件大小及行数

watch -n 2 'mysql -uroot -p'"'"'iam59!z$'"'"' -e "SELECT COUNT(*) FROM 数据库名.user"'
---持续监测数据库表记录
✅ 调试时：通过 trace-id 查看完整调用链

✅ 监控时：通过 metrics 查看性能指标

✅ 审计时：通过 audit log 满足合规要求

✅ 排错时：通过 error log 快速定位问题

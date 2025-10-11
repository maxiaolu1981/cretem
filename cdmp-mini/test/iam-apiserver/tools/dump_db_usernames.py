#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""导出 iam.user 表用户名列表到文件。

方便在 delete-force 测试前后生成基线快照供校验脚本使用。

示例：
  python3 dump_db_usernames.py --output ../output/db_baseline_usernames.txt
"""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Optional, Set

import os
import subprocess

DB_HOST = "192.168.10.8"
DB_USER = "root"
DB_PASSWORD = "iam59!z$"
DB_NAME = "iam"
DB_QUERY = "SELECT name FROM `user`;"


class ScriptError(RuntimeError):
    pass


@dataclass
class DBConfig:
    host: str
    user: str
    password: str
    database: str
    query: str = DB_QUERY
    fallback_host: Optional[str] = None
    unix_socket: Optional[str] = None


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="导出 iam.user 用户名列表")
    parser.add_argument(
        "--output",
        type=Path,
        required=True,
        help="输出文件路径",
    )
    parser.add_argument(
        "--db-host",
        default=DB_HOST,
        help="MySQL 主机地址",
    )
    parser.add_argument(
        "--db-fallback-host",
        default=None,
        help="MySQL 备用主机地址",
    )
    parser.add_argument(
        "--db-user",
        default=DB_USER,
        help="MySQL 用户名",
    )
    parser.add_argument(
        "--db-password",
        default=DB_PASSWORD,
        help="MySQL 密码",
    )
    parser.add_argument(
        "--db-name",
        default=DB_NAME,
        help="MySQL 数据库名",
    )
    parser.add_argument(
        "--db-query",
        default=DB_QUERY,
        help="用于导出的查询语句",
    )
    parser.add_argument(
        "--db-unix-socket",
        default=None,
        help="MySQL UNIX socket 路径 (可选)",
    )
    return parser.parse_args()


def _connect_mysql(config: DBConfig):
    connector_errors = []
    available_drivers = []

    try:
        import pymysql  # type: ignore

        def connect_with_pymysql(host: str):
            kwargs = {
                "host": host,
                "user": config.user,
                "password": config.password,
                "database": config.database,
                "charset": "utf8mb4",
                "cursorclass": pymysql.cursors.Cursor,
            }
            if config.unix_socket and host in {"localhost", "127.0.0.1"}:
                kwargs["unix_socket"] = config.unix_socket
            return pymysql.connect(**kwargs)

        available_drivers.append(("pymysql", connect_with_pymysql))
    except ImportError as exc:
        connector_errors.append(f"pymysql 未安装: {exc}")

    try:
        import mysql.connector  # type: ignore

        def connect_with_mysql_connector(host: str):
            kwargs = {
                "host": host,
                "user": config.user,
                "password": config.password,
                "database": config.database,
            }
            if config.unix_socket and host in {"localhost", "127.0.0.1"}:
                kwargs["unix_socket"] = config.unix_socket
            return mysql.connector.connect(**kwargs)

        available_drivers.append(("mysql.connector", connect_with_mysql_connector))
    except ImportError as exc:
        connector_errors.append(f"mysql-connector-python 未安装: {exc}")

    hosts_to_try = []
    if config.host:
        hosts_to_try.append(config.host)
    if config.fallback_host and config.fallback_host not in hosts_to_try:
        hosts_to_try.append(config.fallback_host)

    for host in hosts_to_try:
        for driver_name, connect_fn in available_drivers:
            try:
                return connect_fn(host)
            except Exception as exc:  # pragma: no cover
                connector_errors.append(f"{driver_name} 连接失败({host}): {exc}")

    hints = []
    if not available_drivers:
        hints.append("请安装 pymysql 或 mysql-connector-python")

    hint_text = ("\n" + "\n".join(hints)) if hints else ""
    raise ScriptError(
        "无法连接 MySQL。" + hint_text + ("\n" + "\n".join(connector_errors) if connector_errors else "")
    )


def fetch_db_usernames(config: DBConfig) -> Set[str]:
    conn = _connect_mysql(config)
    try:
        cursor = conn.cursor()
        try:
            cursor.execute(config.query)
            rows = cursor.fetchall()
        finally:
            cursor.close()
    finally:
        try:
            conn.close()
        except Exception:
            pass
    return {row[0] for row in rows if row and row[0]}


def write_usernames(usernames: Set[str], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fout:
        for name in sorted(usernames):
            fout.write(f"{name}\n")


def main() -> int:
    args = parse_args()

    try:
        config = DBConfig(
            host=args.db_host,
            user=args.db_user,
            password=args.db_password,
            database=args.db_name,
            query=args.db_query,
            fallback_host=args.db_fallback_host,
            unix_socket=args.db_unix_socket,
        )

        usernames = fetch_db_usernames(config)
        write_usernames(usernames, args.output.resolve())
        print(f"✅ 已导出 {len(usernames)} 个用户名 -> {args.output}")
        return 0
    except ScriptError as exc:
        print(f"[错误] {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())

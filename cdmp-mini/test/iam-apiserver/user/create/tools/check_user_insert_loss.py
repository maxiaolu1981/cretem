#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""用户创建用例插入校验工具。

该脚本执行以下步骤：

1. 读取本次测试用例成功写入的用户名（success_usernames.txt）。
2. 连接 192.168.10.8 (root/iam59!z$) MySQL 实例的 iam 库，导出 user 表所有用户名到
   test/iam-apiserver/user/create/output/udatabase_usernames.txt，并剔除内置账号（如 admin）。
3. 对比测试记录、数据库、Redis 缓存三份数据，找出缺失/多余的用户，输出统计
   并根据要求生成 missing_usernames.txt。
4. 如果存在缺失用户，调用 find_missing_user_log.py 从
   /var/log/iam/iam-apiserver.log 中提取疑似问题日志，并给出简单分析建议。
5. 当测试结果和数据库、Redis 均不一致时以非零状态退出，用于在 CI 中快速发现问题。

用法：
  python3 check_user_insert_loss.py [--success-file success_usernames.txt]

可选参数可通过 --help 查看。
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Optional, Sequence, Set, Tuple


# --------------------------- 常量配置 ---------------------------
DB_HOST = "192.168.10.8"
DB_USER = "root"
DB_PASSWORD = "iam59!z$"
DB_NAME = "iam"
DB_QUERY = "SELECT name FROM `user`;"

REDIS_HOST = os.environ.get("TEST_REDIS_HOST", "192.168.10.14")
REDIS_PORT = int(os.environ.get("TEST_REDIS_PORT", "6379"))
REDIS_PASSWORD = os.environ.get("TEST_REDIS_PASSWORD", "")
REDIS_PATTERN = os.environ.get("TEST_REDIS_PATTERN", "user:*")

BASELINE_USERS = {"admin"}

TOOLS_DIR = Path(__file__).resolve().parent
DEFAULT_LOG_PATH = Path("/var/log/iam/iam-apiserver.log")


class ScriptError(RuntimeError):
    """自定义异常便于统一处理错误输出。"""


@dataclass
class DBConfig:
    host: str
    user: str
    password: str
    database: str
    query: str = DB_QUERY
    fallback_host: Optional[str] = None
    unix_socket: Optional[str] = None


@dataclass
class RedisConfig:
    endpoints: Tuple[Tuple[str, int], ...]
    pattern: str
    password: str = ""


@dataclass
class PathConfig:
    output_dir: Path
    db_dump: Path
    missing_file: Path
    sample_missing_file: Path
    log_script: Path


def load_success_usernames(path: Path) -> Set[str]:
    if not path.exists():
        raise ScriptError(f"找不到成功用户名文件: {path}")
    with path.open("r", encoding="utf-8") as f:
        data = {line.strip() for line in f if line.strip()}
    if not data:
        raise ScriptError(f"文件 {path} 为空，无法进行比对")
    return data


def ensure_directory(directory: Path) -> None:
    directory.mkdir(parents=True, exist_ok=True)


def _build_redis_endpoints(hosts_value: str, ports_value: str) -> Tuple[Tuple[str, int], ...]:
    hosts = [host.strip() for host in str(hosts_value).split(",") if host.strip()]
    if not hosts:
        hosts = [REDIS_HOST]

    port_tokens = [token.strip() for token in str(ports_value).split(",") if token.strip()]
    if not port_tokens:
        port_tokens = [str(REDIS_PORT)]

    ports: list[int] = []
    for token in port_tokens:
        try:
            ports.append(int(token))
        except ValueError as exc:
            raise ScriptError(f"无法解析 Redis 端口: {token}") from exc

    endpoints: dict[Tuple[str, int], None] = {}
    for host in hosts:
        for port in ports:
            endpoints[(host, port)] = None

    if not endpoints:
        raise ScriptError("未找到可用的 Redis 地址，请检查 --redis-host / --redis-port 参数")

    return tuple(endpoints.keys())


def _connect_mysql(config: DBConfig):
    """尝试导入 MySQL 客户端库并返回连接对象。"""

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
    if any("1045" in msg for msg in connector_errors):
        hints.append("确认 root 账号允许从当前主机访问，或尝试 --db-host localhost / --db-user iam")

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
        except Exception:  # pragma: no cover - 防御性关闭
            pass

    usernames = {row[0] for row in rows if row and row[0]}
    return usernames


def write_db_dump(usernames: Iterable[str], path: Path) -> None:
    ensure_directory(path.parent)
    with path.open("w", encoding="utf-8") as fout:
        for name in sorted(usernames):
            fout.write(f"{name}\n")


def load_redis_usernames(config: RedisConfig) -> Set[str]:
    """加载 Redis 中 user:* 缓存的用户名，支持多主机尝试并增强错误提示。"""

    errors: list[str] = []
    redis_module = None
    redis_import_error: Optional[Exception] = None

    try:
        import redis  # type: ignore

        redis_module = redis
    except ImportError as exc:
        redis_import_error = exc

    def _load_with_python(host: str, port: int) -> Set[str]:
        assert redis_module is not None
        client = redis_module.StrictRedis(
            host=host,
            port=port,
            password=config.password or None,
            decode_responses=True,
        )
        usernames = set()
        for key in client.scan_iter(match=config.pattern):
            username = key.split(":", 1)[-1]
            if username:
                usernames.add(username)
        return usernames

    def _load_with_cli(host: str, port: int) -> Set[str]:
        cmd = [
            "redis-cli",
            "-h",
            host,
            "-p",
            str(port),
            "keys",
            config.pattern,
        ]
        if config.password:
            cmd.extend(["-a", config.password])
        try:
            result = subprocess.run(
                cmd,
                check=True,
                capture_output=True,
                text=True,
            )
        except FileNotFoundError as exc:
            raise ScriptError(
                "redis 库未安装且未找到 redis-cli，请安装 redis-py 或 redis-cli"
            ) from exc
        except subprocess.CalledProcessError as exc:
            stderr = exc.stderr.strip() if exc.stderr else ""
            hint = f"请确认 redis 服务已启动且可从当前主机访问: {host}:{port}"
            if "NOAUTH" in stderr.upper():
                hint = "Redis 需要密码，请通过 --redis-password 指定"
            raise ScriptError(
                "redis-cli 执行失败: "
                + (stderr or str(exc))
                + f"\n命令: {' '.join(cmd)}"
                + f"\n建议: {hint}"
            ) from exc

        usernames = set()
        for line in result.stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            username = line.split(":", 1)[-1]
            if username:
                usernames.add(username)
        return usernames

    collected_users: Set[str] = set()
    connected_any = False

    for host, port in config.endpoints:
        endpoint_success = False
        if redis_module is not None:
            try:
                collected_users.update(_load_with_python(host, port))
                endpoint_success = True
            except Exception as exc:  # pragma: no cover - 网络异常
                errors.append(f"redis 库访问失败({host}:{port}): {exc}")

        if not endpoint_success:
            try:
                collected_users.update(_load_with_cli(host, port))
                endpoint_success = True
            except ScriptError as exc:
                errors.append(str(exc))
            except Exception as exc:  # pragma: no cover - 防御性
                errors.append(f"redis-cli 调用异常({host}:{port}): {exc}")

        connected_any = connected_any or endpoint_success

    if connected_any:
        return collected_users

    hint_lines = []
    if redis_import_error:
        hint_lines.append(f"redis 库未安装: {redis_import_error}")
    hint_lines.append("请确认以下 Redis 地址在本机可访问并开放对应端口:")
    for host, port in config.endpoints:
        hint_lines.append(f"  - {host}:{port}")

    error_text = "\n".join(errors) if errors else "未知原因"
    raise ScriptError("Redis 读取失败:\n" + error_text + "\n" + "\n".join(hint_lines))


def compute_differences(
    success: Set[str],
    db: Set[str],
    redis_users: Set[str],
) -> Tuple[Set[str], Set[str], Set[str], Set[str]]:
    """返回 (lost_db, extra_db, lost_redis, extra_redis)。"""

    lost_db = success - db
    extra_db = db - success
    lost_redis = success - redis_users
    extra_redis = redis_users - success
    return lost_db, extra_db, lost_redis, extra_redis


def save_missing_usernames(usernames: Sequence[str], path: Path) -> None:
    ensure_directory(path.parent)
    with path.open("w", encoding="utf-8") as fout:
        for name in usernames:
            fout.write(name + "\n")


def run_find_missing_log(
    missing_users: Sequence[str],
    sample_file: Path,
    log_path: Path,
    script_path: Path,
) -> Tuple[str, Counter]:
    ensure_directory(sample_file.parent)
    with sample_file.open("w", encoding="utf-8") as fout:
        for user in missing_users:
            fout.write(user + "\n")

    if not script_path.exists():
        raise ScriptError(f"缺少日志分析脚本: {script_path}")

    if not log_path.exists():
        raise ScriptError(f"日志文件不存在: {log_path}")

    proc = subprocess.run(
        [
            sys.executable,
            str(script_path),
            str(sample_file),
            str(log_path),
        ],
        capture_output=True,
        text=True,
    )

    if proc.returncode != 0:
        raise ScriptError(
            "find_missing_user_log.py 执行失败:\n" + proc.stderr.strip()
        )

    lines = [line for line in proc.stdout.splitlines() if line.strip()]
    severity_counter = Counter()
    for line in lines:
        lowered = line.lower()
        if any(keyword in lowered for keyword in ("error", "失败", "exception")):
            severity_counter["error"] += 1
        elif any(keyword in lowered for keyword in ("warn", "warning", "警告")):
            severity_counter["warning"] += 1
        elif any(keyword in lowered for keyword in ("success", "成功")):
            severity_counter["success"] += 1
        else:
            severity_counter["info"] += 1

    return proc.stdout, severity_counter


def conclude(success: Set[str], db: Set[str], redis_users: Set[str]) -> bool:
    success_count = len(success)
    db_count = len(db)
    redis_count = len(redis_users)

    is_ok = True
    if success_count != db_count and success_count != redis_count:
        print(
            "[严重] 测试记录数与数据库、Redis 均不一致，判定此次测试失败",
            file=sys.stderr,
        )
        is_ok = False
    elif success_count != db_count:
        print(
            "[警告] 测试记录数与数据库不一致，请进一步排查",
            file=sys.stderr,
        )
    elif success_count != redis_count:
        print(
            "[警告] 测试记录数与 Redis 不一致，可能存在缓存写入异常",
            file=sys.stderr,
        )
    else:
        print("[通过] 测试记录、数据库、Redis 一致")

    return is_ok


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="用户插入校验工具")
    parser.add_argument(
        "--success-file",
        type=Path,
        default=Path("success_usernames.txt"),
        help="测试结束后写入的成功用户名列表",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=TOOLS_DIR.parent / "output",
        help="结果输出目录 (默认: 工具目录上一级的 output)",
    )
    parser.add_argument(
        "--db-dump",
        type=Path,
        default=None,
        help="导出数据库用户名的文件路径 (默认: output-dir/udatabase_usernames.txt)",
    )
    parser.add_argument(
        "--db-host",
        default=DB_HOST,
        help="MySQL 主机地址 (默认 192.168.10.8)",
    )
    parser.add_argument(
        "--db-fallback-host",
        default=None,
        help="MySQL 连接失败时的备用主机 (可选)",
    )
    parser.add_argument(
        "--db-user",
        default=DB_USER,
        help="MySQL 用户名 (默认 root)",
    )
    parser.add_argument(
        "--db-password",
        default=DB_PASSWORD,
        help="MySQL 密码",
    )
    parser.add_argument(
        "--db-name",
        default=DB_NAME,
        help="MySQL 数据库名 (默认 iam)",
    )
    parser.add_argument(
        "--db-query",
        default=DB_QUERY,
        help="用于查询用户名的 SQL (默认 SELECT name FROM `user`;)",
    )
    parser.add_argument(
        "--db-unix-socket",
        default=None,
        help="MySQL UNIX socket 路径 (可选)",
    )
    parser.add_argument(
        "--log-file",
        type=Path,
        default=DEFAULT_LOG_PATH,
        help="用于日志分析的 iam-apiserver 日志路径",
    )
    parser.add_argument(
        "--analysis-users",
        type=int,
        default=1,
        help="缺失用户时用于日志分析的样本数量 (>=1)",
    )
    parser.add_argument(
        "--dump-json",
        type=Path,
        default=None,
        help="将最终统计信息写入 JSON 文件 (可选)",
    )
    parser.add_argument(
        "--redis-host",
        default=REDIS_HOST,
        help="Redis 主机地址，支持以逗号分隔的多个候选值",
    )
    parser.add_argument(
        "--redis-port",
        default=str(REDIS_PORT),
        help="Redis 端口，支持以逗号分隔的多个端口",
    )
    parser.add_argument(
        "--redis-password",
        default=REDIS_PASSWORD,
        help="Redis 密码 (如无则留空)",
    )
    parser.add_argument(
        "--redis-pattern",
        default=REDIS_PATTERN,
        help="Redis key 匹配模式 (默认 user:*)",
    )
    parser.add_argument(
        "--missing-file",
        type=Path,
        default=None,
        help="缺失用户名输出文件路径 (默认: output-dir/missing_usernames.txt)",
    )
    parser.add_argument(
        "--sample-missing-file",
        type=Path,
        default=None,
        help="日志分析使用的缺失用户名样本文件 (默认: output-dir/missing_usernames_sample.txt)",
    )
    parser.add_argument(
        "--log-script",
        type=Path,
        default=TOOLS_DIR / "find_missing_user_log.py",
        help="日志分析脚本路径",
    )
    return parser.parse_args()


def dump_summary(
    *,
    success: Set[str],
    db: Set[str],
    redis_users: Set[str],
    lost_db: Set[str],
    extra_db: Set[str],
    lost_redis: Set[str],
    extra_redis: Set[str],
    log_analysis: str | None,
    severity: Counter | None,
    json_path: Path | None,
    paths: PathConfig,
) -> None:
    print(f"测试用例记录成功 username 总数: {len(success)}")
    print(f"数据库实际 username 总数(排除基础账号): {len(db)}")
    print(f"Redis 缓存 username 总数: {len(redis_users)}")

    if lost_db:
        print(f"数据库缺失 username 数: {len(lost_db)}")
        for idx, name in enumerate(sorted(lost_db)[:20], start=1):
            print(f"  缺失[{idx}]: {name}")
        save_missing_usernames(sorted(lost_db), paths.missing_file)
        print(
            f"已将缺失用户写入 {paths.missing_file}，共 {len(lost_db)} 条",
        )
    else:
        print("数据库未缺失任何测试用例成功 username")

    if extra_db:
        print(f"数据库多余 username 数: {len(extra_db)} (非本次测试产生)")

    if lost_redis:
        print(f"Redis 缺失 username 数: {len(lost_redis)}")
    if extra_redis:
        print(f"Redis 多余 username 数: {len(extra_redis)}")

    if log_analysis:
        print("\n===== 缺失用户日志片段 =====")
        print(log_analysis.strip())
        if severity:
            print("\n===== 日志严重度统计 =====")
            for key, value in severity.items():
                print(f"  {key}: {value}")

    if json_path:
        ensure_directory(json_path.parent)
        summary = {
            "success_total": len(success),
            "db_total": len(db),
            "redis_total": len(redis_users),
            "lost_db": sorted(lost_db),
            "extra_db": sorted(extra_db),
            "lost_redis": sorted(lost_redis),
            "extra_redis": sorted(extra_redis),
            "log_severity": severity or {},
        }
        with json_path.open("w", encoding="utf-8") as fout:
            json.dump(summary, fout, ensure_ascii=False, indent=2)


def main() -> int:
    args = parse_args()

    try:
        output_dir = args.output_dir.resolve()
        db_dump_path = args.db_dump.resolve() if args.db_dump else (output_dir / "udatabase_usernames.txt")
        missing_file_path = (
            args.missing_file.resolve() if args.missing_file else (output_dir / "missing_usernames.txt")
        )
        sample_missing_file_path = (
            args.sample_missing_file.resolve()
            if args.sample_missing_file
            else (output_dir / "missing_usernames_sample.txt")
        )
        log_script_path = args.log_script.resolve()

        paths = PathConfig(
            output_dir=output_dir,
            db_dump=db_dump_path,
            missing_file=missing_file_path,
            sample_missing_file=sample_missing_file_path,
            log_script=log_script_path,
        )

        db_config = DBConfig(
            host=args.db_host,
            user=args.db_user,
            password=args.db_password,
            database=args.db_name,
            query=args.db_query,
            fallback_host=args.db_fallback_host,
            unix_socket=args.db_unix_socket,
        )
        redis_config = RedisConfig(
            endpoints=_build_redis_endpoints(args.redis_host, args.redis_port),
            pattern=args.redis_pattern,
            password=args.redis_password,
        )

        success_users = load_success_usernames(args.success_file)
        db_users_raw = fetch_db_usernames(db_config)
        db_users = db_users_raw - BASELINE_USERS
        write_db_dump(db_users_raw, paths.db_dump)
        redis_users = load_redis_usernames(redis_config)

        lost_db, extra_db, lost_redis, extra_redis = compute_differences(
            success_users,
            db_users,
            redis_users,
        )

        log_analysis = None
        severity = None
        if lost_db:
            sample_count = max(1, args.analysis_users)
            sample_users = list(sorted(lost_db))[:sample_count]
            log_analysis, severity = run_find_missing_log(
                sample_users,
                paths.sample_missing_file,
                args.log_file,
                paths.log_script,
            )

        json_path = args.dump_json.resolve() if args.dump_json else None
        dump_summary(
            success=success_users,
            db=db_users,
            redis_users=redis_users,
            lost_db=lost_db,
            extra_db=extra_db,
            lost_redis=lost_redis,
            extra_redis=extra_redis,
            log_analysis=log_analysis,
            severity=severity,
            json_path=json_path,
            paths=paths,
        )

        ok = conclude(success_users, db_users, redis_users)
        return 0 if ok else 1
    except ScriptError as exc:
        print(f"[错误] {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())

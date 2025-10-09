#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""删除用例结果校验工具。

该脚本用于在 delete-force 测试完成后，对照期望删除的用户列表，校验
数据库与 Redis 的最终状态是否符合预期：

1. 所有在目标列表中的用户应当从数据库删除。
2. 除目标列表外的用户不应被误删。
3. 目标用户在 Redis 中的缓存条目应被清理干净。

用法示例（需按实际路径调整）：
    python3 check_user_force_delete.py \
        --target-file ../output/target_usernames.txt \
        --baseline-file ../output/db_baseline_usernames.txt

可选参数可通过 --help 查看。
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Optional, Sequence, Set, Tuple


# --------------------------- 常量配置 ---------------------------
DB_HOST = "127.0.0.1"
DB_USER = "root"
DB_PASSWORD = "iam59!z$"
DB_NAME = "iam"
DB_QUERY = "SELECT name FROM `user`;"

REDIS_HOST = os.environ.get("TEST_REDIS_HOST", "192.168.10.14")
REDIS_PORT = int(os.environ.get("TEST_REDIS_PORT", "6379"))
REDIS_PASSWORD = os.environ.get("TEST_REDIS_PASSWORD", "")
REDIS_PATTERN = os.environ.get("TEST_REDIS_PATTERN", "genericapiserver:user*")

TOOLS_DIR = Path(__file__).resolve().parent


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
    db_dump_after: Path
    undeleted_file: Path
    unexpected_missing_file: Path
    unexpected_extra_file: Path
    redis_residual_file: Path
    json_file: Optional[Path]


def load_usernames_from_file(path: Path, description: str) -> Set[str]:
    if not path.exists():
        raise ScriptError(f"找不到{description}文件: {path}")
    with path.open("r", encoding="utf-8") as f:
        data = {line.strip() for line in f if line.strip()}
    if not data:
        raise ScriptError(f"{description}文件 {path} 为空")
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
        hints.append("确认数据库账号允许从当前主机访问，或尝试 --db-host localhost / --db-user iam")

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

    return {row[0] for row in rows if row and row[0]}


def write_db_dump(usernames: Iterable[str], path: Path) -> None:
    ensure_directory(path.parent)
    with path.open("w", encoding="utf-8") as fout:
        for name in sorted(usernames):
            fout.write(f"{name}\n")


def load_redis_usernames(config: RedisConfig) -> Set[str]:
    """加载 Redis 中 pattern 匹配的用户名集合，支持多主机。"""

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
            raise ScriptError("redis 库未安装且未找到 redis-cli，请安装 redis-py 或 redis-cli") from exc
        except subprocess.CalledProcessError as exc:
            stderr = exc.stderr.strip() if exc.stderr else ""
            hint = f"请确认 redis 服务已启动且可从当前主机访问: {host}:{port}"
            if "NOAUTH" in stderr.upper():
                hint = "Redis 需要密码，请通过 --redis-password 指定"
            raise ScriptError(
                "redis-cli 执行失败: " + (stderr or str(exc)) + f"\n命令: {' '.join(cmd)}\n建议: {hint}"
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


def save_user_list(usernames: Sequence[str], path: Path) -> None:
    ensure_directory(path.parent)
    with path.open("w", encoding="utf-8") as fout:
        for name in sorted(usernames):
            fout.write(name + "\n")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="delete-force 删除结果校验工具")
    parser.add_argument(
        "--target-file",
        type=Path,
        default=Path("../output/target_usernames.txt"),
        help="本次 delete-force 预期删除的用户名列表文件",
    )
    parser.add_argument(
        "--baseline-file",
        type=Path,
        required=True,
        help="删除前数据库用户名快照文件（用于检测误删）",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=TOOLS_DIR.parent / "output",
        help="校验结果输出目录",
    )
    parser.add_argument(
        "--db-dump-after",
        type=Path,
        default=None,
        help="将当前数据库用户名导出到指定文件 (默认: output-dir/udatabase_usernames_after.txt)",
    )
    parser.add_argument(
        "--undeleted-file",
        type=Path,
        default=None,
        help="保存未删除成功的用户名列表 (默认: output-dir/undeleted_usernames.txt)",
    )
    parser.add_argument(
        "--unexpected-missing-file",
        type=Path,
        default=None,
        help="保存被误删的用户名列表 (默认: output-dir/unexpected_missing_usernames.txt)",
    )
    parser.add_argument(
        "--unexpected-extra-file",
        type=Path,
        default=None,
        help="保存数据库中额外出现的用户名列表 (默认: output-dir/unexpected_extra_usernames.txt)",
    )
    parser.add_argument(
        "--redis-residual-file",
        type=Path,
        default=None,
        help="保存 Redis 中残留缓存的用户名列表 (默认: output-dir/redis_residual_usernames.txt)",
    )
    parser.add_argument(
        "--dump-json",
        type=Path,
        default=None,
        help="将最终统计信息写入 JSON 文件 (可选)",
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
        help="Redis key 匹配模式 (默认 genericapiserver:user*)",
    )
    parser.add_argument(
        "--ignore-user",
        action="append",
        default=[],
        help="忽略指定用户名，可重复使用",
    )
    return parser.parse_args()


def dump_summary(
    *,
    target_users: Set[str],
    baseline_users: Set[str],
    db_expected_after: Set[str],
    db_current: Set[str],
    undeleted: Set[str],
    unexpected_missing: Set[str],
    unexpected_extra: Set[str],
    redis_residual: Set[str],
    missing_in_baseline: Set[str],
    paths: PathConfig,
) -> None:
    print(f"目标删除用户数: {len(target_users)}")
    print(f"基线用户数: {len(baseline_users)}")
    print(f"期望最终数据库用户数: {len(db_expected_after)}")
    print(f"实际最终数据库用户数: {len(db_current)}")

    if missing_in_baseline:
        print(f"[警告] 目标用户中有 {len(missing_in_baseline)} 个不在基线文件中，无法验证是否误删：")
        for idx, name in enumerate(sorted(missing_in_baseline), start=1):
            print(f"  缺少基线[{idx}]: {name}")
        print("  👉 如果基线文件是在预创建之前导出的，可忽略此警告；否则请重新生成基线。")

    if undeleted:
        print(f"[错误] 仍存在 {len(undeleted)} 个用户未删除:")
        for idx, name in enumerate(sorted(undeleted)[:20], start=1):
            print(f"  未删除[{idx}]: {name}")
        save_user_list(sorted(undeleted), paths.undeleted_file)
        print(f"  详情已写入 {paths.undeleted_file}")
    else:
        print("✅ 所有目标用户均已从数据库删除")

    if unexpected_missing:
        print(f"[错误] 发现 {len(unexpected_missing)} 个非目标用户被误删:")
        for idx, name in enumerate(sorted(unexpected_missing)[:20], start=1):
            print(f"  误删[{idx}]: {name}")
        save_user_list(sorted(unexpected_missing), paths.unexpected_missing_file)
        print(f"  详情已写入 {paths.unexpected_missing_file}")
    else:
        print("✅ 未发现误删的非目标用户")

    if unexpected_extra:
        print(f"[警告] 数据库中额外存在 {len(unexpected_extra)} 个未在基线中的用户:")
        for idx, name in enumerate(sorted(unexpected_extra)[:20], start=1):
            print(f"  额外[{idx}]: {name}")
        save_user_list(sorted(unexpected_extra), paths.unexpected_extra_file)
        print(f"  详情已写入 {paths.unexpected_extra_file}")
    else:
        print("✅ 数据库未出现额外用户")

    if redis_residual:
        print(f"[错误] 仍有 {len(redis_residual)} 个用户的 Redis 缓存残留:")
        for idx, name in enumerate(sorted(redis_residual)[:20], start=1):
            print(f"  残留[{idx}]: {name}")
        save_user_list(sorted(redis_residual), paths.redis_residual_file)
        print(f"  详情已写入 {paths.redis_residual_file}")
    else:
        print("✅ Redis 中未发现目标用户缓存残留")

    if paths.json_file:
        ensure_directory(paths.json_file.parent)
        summary = {
            "target_total": len(target_users),
            "baseline_total": len(baseline_users),
            "expected_db_total": len(db_expected_after),
            "actual_db_total": len(db_current),
            "undeleted": sorted(undeleted),
            "unexpected_missing": sorted(unexpected_missing),
            "unexpected_extra": sorted(unexpected_extra),
            "redis_residual": sorted(redis_residual),
            "missing_in_baseline": sorted(missing_in_baseline),
        }
        with paths.json_file.open("w", encoding="utf-8") as fout:
            json.dump(summary, fout, ensure_ascii=False, indent=2)
        print(f"📄 JSON 统计已写入 {paths.json_file}")


def main() -> int:
    args = parse_args()

    try:
        output_dir = args.output_dir.resolve()
        db_dump_after = args.db_dump_after.resolve() if args.db_dump_after else (output_dir / "udatabase_usernames_after.txt")
        undeleted_file = args.undeleted_file.resolve() if args.undeleted_file else (output_dir / "undeleted_usernames.txt")
        unexpected_missing_file = (
            args.unexpected_missing_file.resolve()
            if args.unexpected_missing_file
            else (output_dir / "unexpected_missing_usernames.txt")
        )
        unexpected_extra_file = (
            args.unexpected_extra_file.resolve()
            if args.unexpected_extra_file
            else (output_dir / "unexpected_extra_usernames.txt")
        )
        redis_residual_file = (
            args.redis_residual_file.resolve()
            if args.redis_residual_file
            else (output_dir / "redis_residual_usernames.txt")
        )
        json_file = args.dump_json.resolve() if args.dump_json else None

        paths = PathConfig(
            output_dir=output_dir,
            db_dump_after=db_dump_after,
            undeleted_file=undeleted_file,
            unexpected_missing_file=unexpected_missing_file,
            unexpected_extra_file=unexpected_extra_file,
            redis_residual_file=redis_residual_file,
            json_file=json_file,
        )

        target_path = args.target_file.resolve()
        baseline_path = args.baseline_file.resolve()

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

        target_users = load_usernames_from_file(target_path, "目标删除用户")
        baseline_users = load_usernames_from_file(baseline_path, "基线用户")

        db_users_after = fetch_db_usernames(db_config)
        write_db_dump(db_users_after, paths.db_dump_after)

        redis_users = load_redis_usernames(redis_config)

        ignore_users = {user.strip() for user in args.ignore_user if user and user.strip()}
        if ignore_users:
            target_users -= ignore_users
            baseline_users -= ignore_users
            db_users_after -= ignore_users
            redis_users -= ignore_users

        missing_in_baseline = target_users - baseline_users
        db_expected_after = baseline_users - target_users

        undeleted_users = target_users & db_users_after
        unexpected_missing = db_expected_after - db_users_after
        unexpected_extra = db_users_after - db_expected_after
        redis_residual = target_users & redis_users

        dump_summary(
            target_users=target_users,
            baseline_users=baseline_users,
            db_expected_after=db_expected_after,
            db_current=db_users_after,
            undeleted=undeleted_users,
            unexpected_missing=unexpected_missing,
            unexpected_extra=unexpected_extra,
            redis_residual=redis_residual,
            missing_in_baseline=missing_in_baseline,
            paths=paths,
        )

        ok = not (
            undeleted_users
            or unexpected_missing
            or redis_residual
            or unexpected_extra
        )

        if unexpected_extra:
            print("[警告] 存在额外的数据库用户，如为预期外新增请关注。")

        return 0 if ok else 1
    except ScriptError as exc:
        print(f"[错误] {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())

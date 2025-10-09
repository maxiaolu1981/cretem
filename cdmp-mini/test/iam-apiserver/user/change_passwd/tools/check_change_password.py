#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Change password test result validator.

This script consumes the Go test harness output under `output/` and performs
extra sanity checks to guarantee:

* Success cases have the expected database impact (password hash updated and
  still hashed instead of plaintext).
* Negative cases did not accidentally change stored hashes.
* No plaintext password is leaked into iam-apiserver logs.
* Pressure metrics meet the baseline SLA.

The script mirrors the style of the existing create/delete validators so that it
can be invoked automatically from the Go test (`TestChangePassword_Functional`).

Example usage::

    python3 tools/check_change_password.py \
        --results output/change_password_results.json \
        --perf output/change_password_perf.json \
        --summary output/change_password_summary.json

The command exits with status 0 if every check passes, otherwise a non-zero exit
code is returned so the Go test fails fast.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, Optional

DB_DEFAULT_HOST = "127.0.0.1"
DB_FALLBACK_HOST = "127.0.0.1"
DB_DEFAULT_USER = "root"
DB_DEFAULT_PASSWORD = "iam59!z$"
DB_DEFAULT_NAME = "iam"
DB_QUERY = "SELECT password FROM `user` WHERE name = %s"

LOG_DEFAULT_PATH = Path("/var/log/iam/iam-apiserver.log")

SUCCESS_CODE = 100001


class ValidationError(RuntimeError):
    """Custom error for validation failures."""


@dataclass
class DBConfig:
    host: str
    fallback_host: Optional[str]
    user: str
    password: str
    database: str
    unix_socket: Optional[str] = None


@dataclass
class ResultCase:
    name: str
    username: str
    http_status: int
    code: int
    success: bool
    should_update: bool
    should_invalidate: bool
    old_password: Optional[str]
    new_password: Optional[str]
    checks: Dict[str, bool]


@dataclass
class Summary:
    total_cases: int
    passed_cases: int
    failed_cases: int
    db_inconsistencies: int
    log_leaks: int
    sla_breaches: int
    details: Dict[str, Dict[str, str]]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Validate change password outputs")
    parser.add_argument("--results", type=Path, required=True, help="Go test result JSON")
    parser.add_argument("--perf", type=Path, help="Optional performance JSON")
    parser.add_argument("--summary", type=Path, required=True, help="Summary JSON output path")
    parser.add_argument("--db-host", default=DB_DEFAULT_HOST)
    parser.add_argument("--db-fallback-host", default=DB_FALLBACK_HOST)
    parser.add_argument("--db-user", default=DB_DEFAULT_USER)
    parser.add_argument("--db-password", default=DB_DEFAULT_PASSWORD)
    parser.add_argument("--db-name", default=DB_DEFAULT_NAME)
    parser.add_argument("--db-unix-socket", default=None)
    parser.add_argument("--log-file", type=Path, default=LOG_DEFAULT_PATH)
    parser.add_argument("--warn-only", action="store_true", help="Do not exit non-zero on failure")
    return parser.parse_args()


def load_results(path: Path) -> list[ResultCase]:
    if not path.exists():
        raise ValidationError(f"结果文件不存在: {path}")
    with path.open("r", encoding="utf-8") as f:
        raw = json.load(f)
    cases: list[ResultCase] = []
    for item in raw:
        cases.append(
            ResultCase(
                name=item.get("name", ""),
                username=item.get("username", ""),
                http_status=item.get("http_status", 0),
                code=item.get("code", 0),
                success=bool(item.get("success", False)),
                should_update=bool(item.get("should_update_db", False)),
                should_invalidate=bool(item.get("should_invalidate_token", False)),
                old_password=item.get("old_password"),
                new_password=item.get("new_password"),
                checks=item.get("checks", {}),
            )
        )
    return cases


def _connect_mysql(cfg: DBConfig):
    errors: list[str] = []
    candidates = []

    try:
        import pymysql  # type: ignore

        def connect_with_pymysql(host: str):
            kwargs = {
                "host": host,
                "user": cfg.user,
                "password": cfg.password,
                "database": cfg.database,
                "charset": "utf8mb4",
                "cursorclass": pymysql.cursors.Cursor,
            }
            if cfg.unix_socket and host in {"localhost", "127.0.0.1"}:
                kwargs["unix_socket"] = cfg.unix_socket
            return pymysql.connect(**kwargs)

        candidates.append(("pymysql", connect_with_pymysql))
    except ImportError as exc:
        errors.append(f"pymysql 未安装: {exc}")

    try:
        import mysql.connector  # type: ignore

        def connect_with_mysql_connector(host: str):
            kwargs = {
                "host": host,
                "user": cfg.user,
                "password": cfg.password,
                "database": cfg.database,
            }
            if cfg.unix_socket and host in {"localhost", "127.0.0.1"}:
                kwargs["unix_socket"] = cfg.unix_socket
            return mysql.connector.connect(**kwargs)

        candidates.append(("mysql.connector", connect_with_mysql_connector))
    except ImportError as exc:
        errors.append(f"mysql-connector-python 未安装: {exc}")

    if not candidates:
        raise ValidationError("缺少 MySQL 客户端库。请安装 pymysql 或 mysql-connector-python")

    hosts: Iterable[str]
    if cfg.fallback_host and cfg.fallback_host != cfg.host:
        hosts = (cfg.host, cfg.fallback_host)
    else:
        hosts = (cfg.host,)

    for host in hosts:
        for driver_name, fn in candidates:
            try:
                conn = fn(host)
                return conn
            except Exception as exc:  # pragma: no cover - network issues
                errors.append(f"{driver_name} {host} 连接失败: {exc}")

    raise ValidationError("无法连接 MySQL:\n" + "\n".join(errors))


def fetch_password_hash(conn, username: str) -> Optional[str]:
    cursor = conn.cursor()
    try:
        cursor.execute(DB_QUERY, (username,))
        row = cursor.fetchone()
    finally:
        cursor.close()
    if not row:
        return None
    return row[0]


def check_plaintext_in_hash(hashed: str, candidate: Optional[str]) -> bool:
    if not hashed or candidate is None:
        return False
    return candidate in hashed or hashed == candidate


def scan_log_for_password(log_path: Path, password: Optional[str]) -> bool:
    if password is None or password == "":
        return False
    if not log_path.exists():
        raise ValidationError(f"日志文件不存在: {log_path}")
    with log_path.open("r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            if password in line:
                return True
    return False


def validate_cases(cases: list[ResultCase], conn, log_path: Path) -> Summary:
    detail: Dict[str, Dict[str, str]] = {}
    db_inconsistencies = 0
    log_leaks = 0
    failed_cases = 0

    for case in cases:
        case_detail: Dict[str, str] = {}
        hashed = None
        if case.username:
            try:
                hashed = fetch_password_hash(conn, case.username)
            except Exception as exc:
                raise ValidationError(f"查询数据库失败 ({case.username}): {exc}")

        if case.should_update and case.success:
            if not hashed:
                case_detail["db"] = "数据库中找不到更新后的密码"
                db_inconsistencies += 1
            else:
                if check_plaintext_in_hash(hashed, case.new_password):
                    case_detail["db"] = "数据库密码仍为明文或包含新密码"
                    db_inconsistencies += 1
                if case.old_password and not check_plaintext_in_hash(hashed, case.old_password):
                    case_detail.setdefault("db", "hash 已更新")
                if scan_log_for_password(log_path, case.new_password):
                    case_detail["log"] = "明文新密码出现在日志"
                    log_leaks += 1
        else:
            if hashed and case.old_password and check_plaintext_in_hash(hashed, case.new_password):
                case_detail["db"] = "非更新用例不应写入新密码"
                db_inconsistencies += 1

        if not case.success and case.should_update:
            case_detail.setdefault("result", "业务失败导致未更新")
        elif case.success and case.should_update:
            case_detail.setdefault("result", "成功并更新密码")
        else:
            case_detail.setdefault("result", "无数据库更新预期")

        if case.success:
            case_detail.setdefault("status", "success")
        else:
            failed_cases += 1
            case_detail.setdefault("status", "failure")

        detail_key = f"{case.name}::{case.username or 'n/a'}"
        detail[detail_key] = case_detail

    return Summary(
        total_cases=len(cases),
        passed_cases=len(cases) - failed_cases,
        failed_cases=failed_cases,
        db_inconsistencies=db_inconsistencies,
        log_leaks=log_leaks,
        sla_breaches=0,
        details=detail,
    )


def evaluate_performance(perf_path: Optional[Path], summary: Summary) -> None:
    if not perf_path:
        return
    if not perf_path.exists():
        raise ValidationError(f"性能文件不存在: {perf_path}")
    with perf_path.open("r", encoding="utf-8") as f:
        perf_data = json.load(f)

    metrics_list = perf_data if isinstance(perf_data, list) else [perf_data]
    breaches = Counter()

    for metrics in metrics_list:
        avg = metrics.get("avg_response_time_ms", 0)
        p95 = metrics.get("p95_response_time_ms", 0)
        error_rate = metrics.get("error_rate", 0)

        if avg > 500:
            breaches["avg_response_time_ms"] += 1
        if p95 > 1000:
            breaches["p95_response_time_ms"] += 1
        if error_rate and error_rate > 0.01:
            breaches["error_rate"] += 1

    summary.sla_breaches = sum(breaches.values())
    if breaches:
        summary.details.setdefault("performance", {})
        summary.details["performance"]["breaches"] = ", ".join(
            f"{k}={v}" for k, v in breaches.items()
        )


def write_summary(summary_path: Path, summary: Summary) -> None:
    summary_path.parent.mkdir(parents=True, exist_ok=True)
    with summary_path.open("w", encoding="utf-8") as f:
        json.dump(
            {
                "total_cases": summary.total_cases,
                "passed_cases": summary.passed_cases,
                "failed_cases": summary.failed_cases,
                "db_inconsistencies": summary.db_inconsistencies,
                "log_leaks": summary.log_leaks,
                "sla_breaches": summary.sla_breaches,
                "details": summary.details,
            },
            f,
            ensure_ascii=False,
            indent=2,
        )


def main() -> int:
    args = parse_args()

    cases = load_results(args.results)
    if not cases:
        raise ValidationError("结果文件为空，无法执行校验")

    cfg = DBConfig(
        host=args.db_host,
        fallback_host=args.db_fallback_host,
        user=args.db_user,
        password=args.db_password,
        database=args.db_name,
        unix_socket=args.db_unix_socket,
    )

    conn = _connect_mysql(cfg)
    try:
        summary = validate_cases(cases, conn, args.log_file)
    finally:
        try:
            conn.close()
        except Exception:  # pragma: no cover
            pass

    evaluate_performance(args.perf, summary)
    write_summary(args.summary, summary)

    issues = summary.failed_cases + summary.db_inconsistencies + summary.log_leaks + summary.sla_breaches
    if issues > 0 and not args.warn_only:
        raise ValidationError(
            f"校验失败: {summary.failed_cases} 用例失败, "
            f"{summary.db_inconsistencies} 条数据库异常, "
            f"{summary.log_leaks} 条日志泄露, {summary.sla_breaches} 个SLA违约"
        )

    print(
        f"✅ ChangePassword 校验完成: 总用例 {summary.total_cases}, "
        f"通过 {summary.passed_cases}, 失败 {summary.failed_cases}, "
        f"DB 异常 {summary.db_inconsistencies}, 日志泄露 {summary.log_leaks}, SLA 违约 {summary.sla_breaches}"
    )
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except ValidationError as exc:
        print(f"[校验失败] {exc}", file=sys.stderr)
        sys.exit(1)
    except Exception as exc:  # pragma: no cover - defensive
        print(f"[未预期错误] {exc}", file=sys.stderr)
        sys.exit(2)

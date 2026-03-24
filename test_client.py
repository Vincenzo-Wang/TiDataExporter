#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Claw Export Platform API 测试客户端
"""

import requests
import hashlib
import hmac
import time
import json
import argparse
import sys


class ExportClient:
    def __init__(self, base_url, api_key, api_secret):
        self.base_url = base_url
        self.api_key = api_key
        self.api_secret = api_secret

    def _sign(self, method, path, timestamp, body=""):
        body_md5 = hashlib.md5(body.encode()).hexdigest() if body else ""
        sign_str = f"{method}\n{path}\n{timestamp}\n{body_md5}"
        signature = hmac.new(
            self.api_secret.encode(),
            sign_str.encode(),
            hashlib.sha256
        ).hexdigest()
        return signature

    def _headers(self, method, path, body=""):
        timestamp = str(int(time.time()))
        return {
            "Content-Type": "application/json",
            "X-API-Key": self.api_key,
            "X-API-Secret": self.api_secret,
            "X-Timestamp": timestamp,
            "X-Signature": self._sign(method, path, timestamp, body)
        }

    def create_task(self, **params):
        path = "/api/v1/export/tasks"
        body = json.dumps(params)
        resp = requests.post(
            f"{self.base_url}{path}",
            headers=self._headers("POST", path, body),
            data=body
        )
        return resp.json()

    def get_task(self, task_id):
        path = f"/api/v1/export/tasks/{task_id}"
        resp = requests.get(
            f"{self.base_url}{path}",
            headers=self._headers("GET", path)
        )
        return resp.json()

    def cancel_task(self, task_id):
        path = f"/api/v1/export/tasks/{task_id}"
        resp = requests.delete(
            f"{self.base_url}{path}",
            headers=self._headers("DELETE", path)
        )
        return resp.json()

    def download_file(self, task_id, output_path):
        path = f"/api/v1/export/files/{task_id}"
        resp = requests.get(
            f"{self.base_url}{path}",
            headers=self._headers("GET", path),
            allow_redirects=True
        )
        with open(output_path, "wb") as f:
            f.write(resp.content)
        return output_path


def main():
    parser = argparse.ArgumentParser(description="Claw Export Platform API 测试客户端")
    parser.add_argument("--base-url", default="http://localhost:8080", help="API 服务地址")
    parser.add_argument("--api-key", required=True, help="API Key")
    parser.add_argument("--api-secret", required=True, help="API Secret")
    parser.add_argument("--action", choices=["create", "get", "cancel", "download", "poll"], default="create", help="操作类型")
    parser.add_argument("--task-id", help="任务ID (用于 get/cancel/download/poll)")
    parser.add_argument("--tidb-config", default="production-tidb", help="TiDB 配置名称")
    parser.add_argument("--s3-config", default="default-s3", help="S3 配置名称")
    parser.add_argument("--sql", default="SELECT 1", help="SQL 查询语句")
    parser.add_argument("--filetype", default="csv", choices=["sql", "csv"], help="导出文件类型")
    parser.add_argument("--compress", default="gz", choices=["gz", "zstd", "snappy", "none"], help="压缩格式")
    parser.add_argument("--task-name", default="测试导出", help="任务名称")
    parser.add_argument("--output", default="output.tar.gz", help="下载文件保存路径")

    args = parser.parse_args()

    client = ExportClient(
        base_url=args.base_url,
        api_key=args.api_key,
        api_secret=args.api_secret
    )

    if args.action == "create":
        print(f"创建任务: {args.task_name}")
        result = client.create_task(
            tidb_config_name=args.tidb_config,
            s3_config_name=args.s3_config,
            sql_text=args.sql,
            filetype=args.filetype,
            compress=args.compress if args.compress != "none" else None,
            task_name=args.task_name
        )
        print(json.dumps(result, indent=2, ensure_ascii=False))

    elif args.action == "get":
        if not args.task_id:
            print("错误: 需要指定 --task-id")
            sys.exit(1)
        result = client.get_task(args.task_id)
        print(json.dumps(result, indent=2, ensure_ascii=False))

    elif args.action == "cancel":
        if not args.task_id:
            print("错误: 需要指定 --task-id")
            sys.exit(1)
        result = client.cancel_task(args.task_id)
        print(json.dumps(result, indent=2, ensure_ascii=False))

    elif args.action == "download":
        if not args.task_id:
            print("错误: 需要指定 --task-id")
            sys.exit(1)
        output_path = client.download_file(args.task_id, args.output)
        print(f"文件已下载: {output_path}")

    elif args.action == "poll":
        if not args.task_id:
            print("错误: 需要指定 --task-id")
            sys.exit(1)
        print(f"轮询任务状态: {args.task_id}")
        while True:
            task = client.get_task(args.task_id)
            status = task.get("data", {}).get("status", "unknown")
            print(f"状态: {status}")
            print(json.dumps(task, indent=2, ensure_ascii=False))

            if status in ["success", "failed", "canceled"]:
                if status == "success":
                    print("\n任务成功，可以下载文件")
                break
            time.sleep(3)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Claw Export Platform API 测试客户端
"""

import argparse
import hashlib
import hmac
import json
import os
import sys
import time
import urllib.parse

import requests


class ExportClient:
    def __init__(self, base_url, api_key, api_secret):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.api_secret = api_secret

    def _sign(self, method, path, timestamp, body=""):
        body_md5 = hashlib.md5(body.encode()).hexdigest() if body else ""
        sign_str = f"{method}\n{path}\n{timestamp}\n{body_md5}"
        return hmac.new(self.api_secret.encode(), sign_str.encode(), hashlib.sha256).hexdigest()

    def _headers(self, method, path, body=""):
        timestamp = str(int(time.time()))
        return {
            "Content-Type": "application/json",
            "X-API-Key": self.api_key,
            "X-API-Secret": self.api_secret,
            "X-Timestamp": timestamp,
            "X-Signature": self._sign(method, path, timestamp, body),
        }

    def create_task(self, **params):
        path = "/api/v1/export/tasks"
        body = json.dumps(params)
        resp = requests.post(f"{self.base_url}{path}", headers=self._headers("POST", path, body), data=body)
        return resp.json()

    def get_task(self, task_id):
        path = f"/api/v1/export/tasks/{task_id}"
        resp = requests.get(f"{self.base_url}{path}", headers=self._headers("GET", path))
        return resp.json()

    def cancel_task(self, task_id):
        path = f"/api/v1/export/tasks/{task_id}"
        resp = requests.delete(f"{self.base_url}{path}", headers=self._headers("DELETE", path))
        return resp.json()

    @staticmethod
    def _download_url(url, output_file):
        with requests.get(url, stream=True, timeout=120) as resp:
            resp.raise_for_status()
            with open(output_file, "wb") as f:
                for chunk in resp.iter_content(chunk_size=8192):
                    if chunk:
                        f.write(chunk)

    @staticmethod
    def _guess_filename(file_item, fallback):
        name = file_item.get("name") or ""
        if name:
            return name
        parsed = urllib.parse.urlparse(file_item.get("url") or "")
        candidate = os.path.basename(parsed.path)
        return candidate or fallback

    def download_file(self, task_id, output_path, file_index=None):
        path = f"/api/v1/export/files/{task_id}"
        resp = requests.get(
            f"{self.base_url}{path}",
            headers=self._headers("GET", path),
            allow_redirects=False,
            timeout=60,
        )

        if resp.status_code in (301, 302, 303, 307, 308):
            location = resp.headers.get("Location")
            if not location:
                raise RuntimeError("下载重定向缺少 Location")
            self._download_url(location, output_path)
            return [output_path]

        content_type = resp.headers.get("Content-Type", "")
        if "application/json" not in content_type:
            resp.raise_for_status()
            with open(output_path, "wb") as f:
                f.write(resp.content)
            return [output_path]

        payload = resp.json()
        if payload.get("code") != 0:
            raise RuntimeError(f"下载失败: {payload}")

        files = payload.get("data", {}).get("files") or []
        if not files:
            raise RuntimeError("未返回可下载文件")

        if file_index is not None:
            if file_index < 0 or file_index >= len(files):
                raise RuntimeError(f"file_index 超出范围: {file_index}, 总数={len(files)}")
            files = [files[file_index]]

        if len(files) == 1:
            target = output_path
            self._download_url(files[0]["url"], target)
            return [target]

        os.makedirs(output_path, exist_ok=True)
        downloaded = []
        for i, item in enumerate(files):
            filename = self._guess_filename(item, f"file_{i + 1}")
            target = os.path.join(output_path, filename)
            self._download_url(item["url"], target)
            downloaded.append(target)
        return downloaded


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
    parser.add_argument("--output", default="output", help="下载输出路径（单文件=文件路径，多文件=目录路径）")
    parser.add_argument("--file-index", type=int, help="多文件下载时指定文件下标（从 0 开始）")

    args = parser.parse_args()

    client = ExportClient(base_url=args.base_url, api_key=args.api_key, api_secret=args.api_secret)

    if args.action == "create":
        print(f"创建任务: {args.task_name}")
        result = client.create_task(
            tidb_config_name=args.tidb_config,
            s3_config_name=args.s3_config,
            sql_text=args.sql,
            filetype=args.filetype,
            compress=args.compress if args.compress != "none" else None,
            task_name=args.task_name,
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
        outputs = client.download_file(args.task_id, args.output, args.file_index)
        for p in outputs:
            print(f"文件已下载: {p}")

    elif args.action == "poll":
        if not args.task_id:
            print("错误: 需要指定 --task-id")
            sys.exit(1)
        print(f"轮询任务状态: {args.task_id}")
        while True:
            task = client.get_task(args.task_id)
            data = task.get("data", {})
            status = data.get("status", "unknown")
            file_count = data.get("file_count") or len(data.get("files") or [])
            print(f"状态: {status} (file_count={file_count})")
            print(json.dumps(task, indent=2, ensure_ascii=False))

            if status in ["success", "failed", "canceled"]:
                if status == "success":
                    print("\n任务成功，可以下载文件")
                break
            time.sleep(3)


if __name__ == "__main__":
    main()

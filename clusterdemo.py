#!/usr/bin/env python3
import argparse
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request


ROOT = os.path.dirname(os.path.abspath(__file__))


def random_port():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port


def http_json(method, url, payload=None, timeout=10.0):
    data = None
    headers = {}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    with urllib.request.urlopen(request, timeout=timeout) as response:
        body = response.read()
    return json.loads(body.decode("utf-8"))


def wait_for_health(control_addr, timeout=20.0):
    deadline = time.time() + timeout
    last_error = None
    while time.time() < deadline:
        try:
            data = http_json("GET", f"http://{control_addr}/healthz")
            if data.get("status") == "ok":
                return
        except Exception as exc:  # noqa: BLE001
            last_error = exc
        time.sleep(0.2)
    raise RuntimeError(f"node at {control_addr} did not become healthy: {last_error}")


def start_node(repo_root, data_dir, cluster_port, control_port, trusted_admin):
    command = [
        "go",
        "run",
        "./cmd/clusterdemo",
        "--mode",
        "serve",
        "--data-dir",
        data_dir,
        "--cluster-addr",
        f"127.0.0.1:{cluster_port}",
        "--control-addr",
        f"127.0.0.1:{control_port}",
        "--trusted-admin",
        trusted_admin,
    ]
    return subprocess.Popen(
        command,
        cwd=repo_root,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )


def read_pubkey(repo_root, data_dir):
    command = [
        "go",
        "run",
        "./cmd/clusterdemo",
        "--mode",
        "pubkey",
        "--data-dir",
        data_dir,
    ]
    result = subprocess.run(
        command,
        cwd=repo_root,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return result.stdout.strip()


def issue_cert(repo_root, data_dir, admin_key_dir, trusted_admin):
    command = [
        "go",
        "run",
        "./cmd/clusterdemo",
        "--mode",
        "issue-cert",
        "--data-dir",
        data_dir,
        admin_key_dir,
        trusted_admin,
    ]
    subprocess.run(
        command,
        cwd=repo_root,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )


def join_all(nodes):
    for idx, node in enumerate(nodes):
        for jdx, peer in enumerate(nodes):
            if idx == jdx:
                continue
            http_json(
                "POST",
                f"http://{node['control_addr']}/join",
                {
                    "nodeID": peer["peer"]["nodeIDBase64"],
                    "addresses": peer["peer"]["addresses"],
                },
            )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--keep-temp",
        action="store_true",
        help="keep temporary node directories",
    )
    args = parser.parse_args()

    temp_root = tempfile.mkdtemp(prefix="ouroboros-clusterdemo-")
    processes = []
    try:
        data_dirs = [os.path.join(temp_root, f"node-{idx}") for idx in range(3)]
        for data_dir in data_dirs:
            os.makedirs(data_dir, exist_ok=True)

        trusted_admin = read_pubkey(ROOT, data_dirs[0])
        for data_dir in data_dirs:
            issue_cert(ROOT, data_dir, data_dirs[0], trusted_admin)

        nodes = []
        for idx, data_dir in enumerate(data_dirs):
            cluster_port = random_port()
            control_port = random_port()
            process = start_node(
                ROOT,
                data_dir,
                cluster_port,
                control_port,
                trusted_admin,
            )
            processes.append(process)
            control_addr = f"127.0.0.1:{control_port}"
            wait_for_health(control_addr)
            peer = http_json("GET", f"http://{control_addr}/peer")
            nodes.append(
                {
                    "name": f"node-{idx+1}",
                    "data_dir": data_dir,
                    "cluster_port": cluster_port,
                    "control_port": control_port,
                    "control_addr": control_addr,
                    "peer": peer,
                }
            )

        join_all(nodes)

        broadcasts = [
            (nodes[0], "reliable", "hello from node-1"),
            (nodes[1], "reliable", "hello from node-2"),
            (nodes[2], "unreliable", "hello from node-3"),
        ]
        for node, mode, payload in broadcasts:
            result = http_json(
                "POST",
                f"http://{node['control_addr']}/broadcast",
                {
                    "type": 4,
                    "payload": payload,
                    "mode": mode,
                },
            )
            print(f"broadcast from {node['name']} ({mode}): {json.dumps(result)}")

        time.sleep(1.0)

        for node in nodes:
            messages = http_json("GET", f"http://{node['control_addr']}/messages")
            print(f"{node['name']} received:")
            for message in messages.get("messages", []):
                print(f"  {json.dumps(message)}")

        return 0
    finally:
        for process in processes:
            if process.poll() is None:
                process.terminate()
        for process in processes:
            try:
                output, _ = process.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                try:
                    output, _ = process.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    output = ""
            if output:
                sys.stderr.write(output)
        if not args.keep_temp:
            shutil.rmtree(temp_root, ignore_errors=True)
        else:
            print(f"kept temp dir: {temp_root}")


if __name__ == "__main__":
    raise SystemExit(main())

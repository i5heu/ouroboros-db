#!/usr/bin/env python3
# GENERATED, NO HUMAN REVIEW, DO NOT RUN ON PRODUCTION SYSTEMS

import asyncio
import contextlib
import json
import shutil
import tempfile
import argparse
import os
import signal
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical
from textual.message import Message
from textual.widgets import Footer, Header, Input, RichLog, Static

EVENT_PREFIX = "OB_EVT "
NODE_NAMES = ("node1", "node2", "node3")
IGNORED_STDERR_SNIPPETS = (
    "failed to sufficiently increase receive buffer size",
    "quic-go/quic-go/wiki/UDP-Buffer-Sizes",
)


@dataclass
class NodeProcess:
    name: str
    process: asyncio.subprocess.Process
    data_dir: Path
    node_id: str = ""
    listen_addr: str = ""
    ready: asyncio.Event = field(default_factory=asyncio.Event)


class NodeLine(Message):
    def __init__(self, node: str, line: str) -> None:
        super().__init__()
        self.node = node
        self.line = line


class NodeEvent(Message):
    def __init__(self, node: str, payload: dict[str, Any]) -> None:
        super().__init__()
        self.node = node
        self.payload = payload


class ClusterDemoApp(App[None]):
    CSS = """
    Screen {
        layout: vertical;
    }

    #columns {
        height: 1fr;
    }

    .node-col {
        width: 1fr;
        border: round $accent;
        margin: 0 1;
    }

    .node-title {
        height: 1;
        text-style: bold;
        padding-left: 1;
    }

    .node-log {
        height: 1fr;
    }

    #input {
        margin: 1 1;
    }
    """

    BINDINGS = [("ctrl+c", "quit", "Quit")]

    def __init__(self, auto: bool = False, auto_msg: str = "hello-from-auto", auto_kill_delay: float = 0.5) -> None:
        """auto: if True send a message and trigger SIGINT after cluster is ready

        auto_msg: message text to send to `node1`
        auto_kill_delay: seconds to wait after sending before raising SIGINT
        """
        super().__init__()
        self.repo_root = Path(__file__).resolve().parent
        self.binary_path = self.repo_root / "clusterdemo" / "bin" / "clusterdemo"
        self.nodes: dict[str, NodeProcess] = {}
        self.stream_tasks: list[asyncio.Task[Any]] = []

        # auto-run options (used for reproducing Ctrl+C shutdown)
        self._auto = bool(auto)
        self._auto_msg = str(auto_msg)
        self._auto_kill_delay = float(auto_kill_delay)

    def compose(self) -> ComposeResult:
        yield Header()
        with Horizontal(id="columns"):
            for node in NODE_NAMES:
                with Vertical(classes="node-col"):
                    yield Static(node, classes="node-title")
                    yield RichLog(
                        id=f"log-{node}",
                        classes="node-log",
                        wrap=True,
                        highlight=False,
                    )
        yield Input(
            placeholder="Type message for node1 and press Enter",
            id="input",
        )
        yield Footer()

    async def on_mount(self) -> None:
        asyncio.create_task(self.start_cluster())

    async def start_cluster(self) -> None:
        try:
            await self._build_binary()
            await self._spawn_nodes()
            await self._wait_until_ready()
            await self._join_mesh()
            self._write_log("node1", "cluster ready")
            self._write_log("node2", "cluster ready")
            self._write_log("node3", "cluster ready")

            # if auto mode is enabled: send a message and schedule a SIGINT
            if getattr(self, "_auto", False):
                node1 = self.nodes.get("node1")
                if node1 is not None:
                    try:
                        await self._write_stdin(node1, f"/send {self._auto_msg}\n")
                    except Exception:
                        # best-effort; continue to schedule SIGINT
                        pass
                loop = asyncio.get_running_loop()
                loop.call_later(
                    float(self._auto_kill_delay), lambda: os.kill(os.getpid(), signal.SIGINT)
                )
        except Exception as exc:
            self._broadcast(f"startup error: {exc}")

    async def _build_binary(self) -> None:
        self._broadcast("building cmd/clusterdemo-A")
        self.binary_path.parent.mkdir(parents=True, exist_ok=True)

        proc = await asyncio.create_subprocess_exec(
            "go",
            "build",
            "-o",
            str(self.binary_path),
            "./cmd/clusterdemo-A",
            cwd=str(self.repo_root),
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout, stderr = await proc.communicate()
        if proc.returncode != 0:
            msg = stderr.decode().strip() or stdout.decode().strip()
            raise RuntimeError(msg or "go build failed")

    async def _spawn_nodes(self) -> None:
        for name in NODE_NAMES:
            data_dir = Path(
                tempfile.mkdtemp(prefix=f"clusterdemo-A-{name}-")
            )
            cmd = [
                str(self.binary_path),
                "--node-name",
                name,
                "--listen",
                "127.0.0.1:0",
                "--data-dir",
                str(data_dir),
                "--stdin=true",
            ]
            if name == "node1":
                cmd.append("--send-on-stdin=true")

            proc = await asyncio.create_subprocess_exec(
                *cmd,
                cwd=str(self.repo_root),
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )

            runtime = NodeProcess(
                name=name,
                process=proc,
                data_dir=data_dir,
            )
            self.nodes[name] = runtime
            self.stream_tasks.append(
                asyncio.create_task(self._read_stdout(runtime))
            )
            self.stream_tasks.append(
                asyncio.create_task(self._read_stderr(runtime))
            )

    async def _wait_until_ready(self) -> None:
        await asyncio.gather(
            *(self.nodes[name].ready.wait() for name in NODE_NAMES)
        )

    async def _join_mesh(self) -> None:
        for src in NODE_NAMES:
            src_node = self.nodes[src]
            for dst in NODE_NAMES:
                if src == dst:
                    continue
                dst_node = self.nodes[dst]
                await self._write_stdin(
                    src_node,
                    f"/join {dst_node.node_id} {dst_node.listen_addr}\n",
                )

    async def _read_stdout(self, node: NodeProcess) -> None:
        assert node.process.stdout is not None
        while True:
            raw = await node.process.stdout.readline()
            if not raw:
                self.post_message(
                    NodeLine(node.name, "[stdout closed]")
                )
                return

            line = raw.decode(errors="replace").strip()
            if line.startswith(EVENT_PREFIX):
                payload_text = line[len(EVENT_PREFIX) :]
                try:
                    payload = json.loads(payload_text)
                except json.JSONDecodeError:
                    self.post_message(NodeLine(node.name, line))
                    continue
                self.post_message(NodeEvent(node.name, payload))
                continue

            self.post_message(NodeLine(node.name, line))

    async def _read_stderr(self, node: NodeProcess) -> None:
        assert node.process.stderr is not None
        while True:
            raw = await node.process.stderr.readline()
            if not raw:
                self.post_message(
                    NodeLine(node.name, "[stderr closed]")
                )
                return
            line = raw.decode(errors="replace").strip()
            if any(
                snippet in line
                for snippet in IGNORED_STDERR_SNIPPETS
            ):
                continue
            if line:
                self.post_message(NodeLine(node.name, line))

    async def on_node_line(self, message: NodeLine) -> None:
        self._write_log(message.node, message.line)

    async def on_node_event(self, message: NodeEvent) -> None:
        payload = message.payload
        node = self.nodes[message.node]

        event = payload.get("event", "unknown")
        if event == "ready":
            node.node_id = payload.get("nodeId", "")
            node.listen_addr = payload.get("listenAddr", "")
            node.ready.set()

        display = self._format_event_line(payload)
        self._write_log(message.node, display)

    def _format_event_line(self, payload: dict[str, Any]) -> str:
        event = payload.get("event", "unknown")
        peer = payload.get("peerId", "-")
        text = payload.get("text", "-")
        ok = payload.get("success", 0)
        fail = payload.get("failed", 0)
        return (
            f"event:{event} | peer:{peer} | "
            f"text:{text} | ok:{ok} fail:{fail}"
        )

    async def on_input_submitted(self, event: Input.Submitted) -> None:
        text = event.value.strip()
        event.input.value = ""
        if not text:
            return

        node1 = self.nodes.get("node1")
        if node1 is None:
            self._broadcast("node1 is not running")
            return
        await self._write_stdin(node1, f"/send {text}\n")

    async def _write_stdin(self, node: NodeProcess, data: str) -> None:
        if node.process.stdin is None:
            self._write_log(node.name, "stdin is closed")
            return
        node.process.stdin.write(data.encode())
        await node.process.stdin.drain()

    async def on_shutdown(self) -> None:
        await self.stop_cluster()

    async def stop_cluster(self) -> None:
        for task in self.stream_tasks:
            task.cancel()
        await asyncio.gather(
            *self.stream_tasks,
            return_exceptions=True,
        )

        for node in self.nodes.values():
            if node.process.returncode is None:
                with contextlib.suppress(ProcessLookupError):
                    node.process.terminate()

        for node in self.nodes.values():
            if node.process.returncode is not None:
                continue
            try:
                await asyncio.wait_for(node.process.wait(), timeout=2)
            except asyncio.TimeoutError:
                with contextlib.suppress(ProcessLookupError):
                    node.process.kill()
                with contextlib.suppress(Exception):
                    await node.process.wait()

        for node in self.nodes.values():
            shutil.rmtree(node.data_dir, ignore_errors=True)

    def _write_log(self, node: str, line: str) -> None:
        log = self.query_one(f"#log-{node}", RichLog)
        log.write(line)

    def _broadcast(self, line: str) -> None:
        for node in NODE_NAMES:
            self._write_log(node, line)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Run clusterdemo UI (with optional auto-send)")
    parser.add_argument("--auto", action="store_true", help="send message automatically and trigger Ctrl+C")
    parser.add_argument("--auto-msg", default="hello-from-auto", help="message text to send in --auto mode")
    parser.add_argument("--auto-delay", type=float, default=0.5, help="seconds to wait after sending before SIGINT")
    args = parser.parse_args()

    ClusterDemoApp(auto=args.auto, auto_msg=args.auto_msg, auto_kill_delay=args.auto_delay).run()

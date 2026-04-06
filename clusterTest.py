#!/usr/bin/env python3
"""Run multi-node OuroborosDB cluster scenarios against the interactive CLI.

The harness provisions temporary credentials, starts three nodes via
`go run cmd/interactive/main.go`, prefixes live logs per node, and then drives
scenario behavior through the interactive REPL.
"""

from __future__ import annotations

import argparse
import contextlib
from datetime import datetime
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable


MAX_TEST_TIME = 300.0
START_TIMEOUT = 60.0
COMMAND_TIMEOUT = 30.0
MESSAGE_TIMEOUT = 30.0
MESH_TIMEOUT = 10.0
PROCESS_EXIT_TIMEOUT = 10.0
POLL_INTERVAL = 0.1
ANSI_RESET = "\033[0m"
NODE_COLORS = {
    1: "\033[38;5;39m",
    2: "\033[38;5;190m",
    3: "\033[38;5;208m",
}


def log(message: str) -> None:
    """Print a harness-level status line immediately."""
    print(f"{timestamp_now()} {message}", flush=True)


def timestamp_now() -> str:
    """Return a local timestamp with microsecond precision."""
    return datetime.now().strftime("%H:%M:%S.%f")


def should_use_color(color_mode: str) -> bool:
    """Resolve whether ANSI colors should be emitted for node prefixes."""
    if color_mode == "always":
        return True
    if color_mode == "never":
        return False
    return sys.stdout.isatty()


def colorize(text: str, color: str, enabled: bool) -> str:
    """Wrap text in an ANSI color sequence when color output is enabled."""
    if not enabled:
        return text
    return f"{color}{text}{ANSI_RESET}"


def run_command(
    command: list[str],
    cwd: Path,
    timeout: float = COMMAND_TIMEOUT,
) -> subprocess.CompletedProcess[str]:
    """Run a repository command and raise with combined output on failure."""
    result = subprocess.run(
        command,
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=False,
        timeout=timeout,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"command failed ({result.returncode}): {' '.join(command)}\n"
            f"{result.stdout}"
        )
    return result


@dataclass(frozen=True)
class ReceivedMessage:
    """A parsed user-message event observed in one node's live output."""

    from_label: str
    peer_id: str
    text: str


@dataclass
class NodeRuntime:
    """Manage one interactive node process and its parsed runtime events.

    The harness treats the interactive CLI as the test surface. This runtime
    wrapper owns process startup, REPL input, prefixed output streaming, and the
    small amount of output parsing needed for readiness and assertions.
    """

    number: int
    data_dir: Path
    cert_path: Path
    use_color: bool = True
    bootstrap_addresses: list[str] = field(default_factory=list)
    process: subprocess.Popen[str] | None = None
    lines: list[str] = field(default_factory=list)
    received_messages: list[ReceivedMessage] = field(default_factory=list)
    known_peer_ids: set[str] = field(default_factory=set)
    node_id: str | None = None
    listen_address: str | None = None
    start_event: threading.Event = field(default_factory=threading.Event)
    condition: threading.Condition = field(
        default_factory=lambda: threading.Condition(threading.Lock())
    )
    reader_thread: threading.Thread | None = None

    @property
    def prefix(self) -> str:
        """Return the display prefix for this node, optionally colorized."""
        prefix = f"[Node {self.number}]"
        color = NODE_COLORS.get(self.number, "")
        return colorize(prefix, color, self.use_color)

    def command(self) -> list[str]:
        """Build the `go run` command used to start this node."""
        command = [
            "go",
            "run",
            "cmd/interactive/main.go",
            "-data-dir",
            str(self.data_dir),
            "-listen",
            "127.0.0.1:0",
            "-node-cert",
            str(self.cert_path),
        ]
        if self.bootstrap_addresses:
            command.extend([
                "-bootstrap",
                ",".join(self.bootstrap_addresses),
            ])
        return command

    def start(self, cwd: Path) -> None:
        """Launch the interactive node process and start its output reader."""
        if self.process is not None:
            raise RuntimeError(f"node {self.number} already started")
        self.process = subprocess.Popen(
            self.command(),
            cwd=cwd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )
        self.reader_thread = threading.Thread(
            target=self._read_output,
            name=f"node-{self.number}-reader",
            daemon=True,
        )
        self.reader_thread.start()

    def _read_output(self) -> None:
        """Stream node output to the console and feed the event parser."""
        assert self.process is not None
        assert self.process.stdout is not None
        for raw_line in self.process.stdout:
            line = raw_line.rstrip("\n")
            display_line = line.strip()
            if display_line:
                print(
                    f"{timestamp_now()} {self.prefix} {display_line}",
                    flush=True,
                )
            self._record_line(display_line)

    def _record_line(self, line: str) -> None:
        """Normalize one output line and update parsed runtime state."""
        if not line:
            return
        normalized = line
        if normalized.startswith("interactive> "):
            normalized = normalized[len("interactive> ") :].strip()
            if not normalized:
                return

        with self.condition:
            self.lines.append(normalized)

            if normalized.startswith("node started:"):
                self.node_id = normalized.split(":", 1)[1].strip()
            elif normalized.startswith("listening on:"):
                self.listen_address = normalized.split(":", 1)[1].strip()
            elif normalized == "no peers connected":
                self.known_peer_ids.clear()
            else:
                peer_match = re.match(
                    r"^-\s+([0-9a-f]+)\s+",
                    normalized,
                )
                if peer_match is not None:
                    self.known_peer_ids.add(peer_match.group(1))
                else:
                    msg_match = re.match(
                        r"^received from\s+(?P<from>[^\s]+)\s+\((?P<peer>[^)]+)\):\s+(?P<text>.+)$",
                        normalized,
                    )
                    if msg_match is not None:
                        self.received_messages.append(
                            ReceivedMessage(
                                from_label=msg_match.group("from"),
                                peer_id=msg_match.group("peer"),
                                text=msg_match.group("text"),
                            )
                        )

            if self.node_id and self.listen_address:
                self.start_event.set()
            self.condition.notify_all()

    def wait_until_started(self, timeout: float = START_TIMEOUT) -> None:
        """Block until the node reports both its node ID and listen address."""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.start_event.wait(timeout=POLL_INTERVAL):
                return
            if self.process is not None and self.process.poll() is not None:
                raise RuntimeError(
                    f"node {self.number} exited before startup with code "
                    f"{self.process.returncode}\nlast output:\n{self.tail_output()}"
                )

        if not self.start_event.is_set():
            raise RuntimeError(
                f"node {self.number} did not report startup within {timeout:.1f}s\n"
                f"last output:\n{self.tail_output()}"
            )

    def send(self, command: str) -> None:
        """Send one REPL command to the interactive node process."""
        if self.process is None or self.process.stdin is None:
            raise RuntimeError(f"node {self.number} stdin is not available")
        if self.process.poll() is not None:
            raise RuntimeError(
                f"node {self.number} already exited with code {self.process.returncode}\n"
                f"last output:\n{self.tail_output()}"
            )
        self.process.stdin.write(f"{command}\n")
        self.process.stdin.flush()

    def wait_for(
        self,
        predicate: Callable[["NodeRuntime"], bool],
        timeout: float,
        description: str,
    ) -> None:
        """Wait for a parsed state condition while failing fast on process exit."""
        deadline = time.monotonic() + timeout
        with self.condition:
            while not predicate(self):
                if self.process is not None and self.process.poll() is not None:
                    raise RuntimeError(
                        f"node {self.number} exited while waiting for {description} "
                        f"with code {self.process.returncode}\nlast output:\n"
                        f"{self.tail_output()}"
                    )
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise RuntimeError(
                        f"timed out waiting for {description} on node {self.number}\n"
                        f"last output:\n{self.tail_output()}"
                    )
                self.condition.wait(timeout=min(remaining, POLL_INTERVAL))

    def tail_output(self, lines: int = 20) -> str:
        """Return the most recent parsed output lines for error reporting."""
        with self.condition:
            return "\n".join(self.lines[-lines:])

    def shutdown(self) -> None:
        """Stop the node gracefully, then escalate to terminate or kill if needed."""
        if self.process is None:
            return
        if self.process.poll() is None:
            with contextlib.suppress(Exception):
                self.send("quit")
            try:
                self.process.wait(timeout=PROCESS_EXIT_TIMEOUT)
            except subprocess.TimeoutExpired:
                self.process.terminate()
                try:
                    self.process.wait(timeout=PROCESS_EXIT_TIMEOUT)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait(timeout=PROCESS_EXIT_TIMEOUT)
        if self.reader_thread is not None:
            self.reader_thread.join(timeout=1.0)
        with contextlib.suppress(Exception):
            if self.process.stdin is not None:
                self.process.stdin.close()
        with contextlib.suppress(Exception):
            if self.process.stdout is not None:
                self.process.stdout.close()

    def is_running(self) -> bool:
        """Return True while the child process is still alive."""
        return self.process is not None and self.process.poll() is None


class ClusterHarness:
    """Own temporary test state, node lifecycles, and scenario orchestration."""

    def __init__(
        self,
        repo_root: Path,
        keep_temp: bool = False,
        color_mode: str = "auto",
    ) -> None:
        """Create a fresh temporary workspace for one cluster test run."""
        self.repo_root = repo_root
        self.keep_temp = keep_temp
        self.use_color = should_use_color(color_mode)
        self._temp_dir = Path(tempfile.mkdtemp(prefix="ouroboros-cluster-test-"))
        self.admin_ca_path = self._temp_dir / "admin.oukey"
        self.nodes: list[NodeRuntime] = []

    def cleanup(self) -> None:
        """Run final shutdown safety and remove temporary files unless retained."""
        self.shutdown_cluster()
        if self.keep_temp:
            log(f"kept temporary files at {self._temp_dir}")
            return
        shutil.rmtree(self._temp_dir, ignore_errors=True)

    def shutdown_cluster(self) -> None:
        """Stop every node process and verify that none remain running."""
        for node in reversed(self.nodes):
            with contextlib.suppress(Exception):
                node.shutdown()
        still_running = [
            str(node.number)
            for node in self.nodes
            if node.is_running()
        ]
        if still_running:
            raise RuntimeError(
                "failed to stop all node processes: "
                + ", ".join(still_running)
            )

    def provision_certificates(self) -> None:
        """Create a root CA, two user CAs, and the three node certificates."""
        log("Provisioning root CA, user CAs, and node certificates")
        run_command(
            ["go", "run", "cmd/certgen/main.go", "admin-ca", "-out", str(self.admin_ca_path)],
            cwd=self.repo_root,
        )

        user_ca_paths = {
            2: self._temp_dir / "user-node2.oukey",
            3: self._temp_dir / "user-node3.oukey",
        }
        for node_number, user_ca_path in user_ca_paths.items():
            log(f"Provisioning user CA for node {node_number}")
            run_command(
                [
                    "go",
                    "run",
                    "cmd/certgen/main.go",
                    "user-ca",
                    "-admin-key",
                    str(self.admin_ca_path),
                    "-out",
                    str(user_ca_path),
                ],
                cwd=self.repo_root,
            )

        self.nodes = []
        for node_number in (1, 2, 3):
            node_dir = self._temp_dir / f"node{node_number}"
            node_dir.mkdir(parents=True, exist_ok=True)
            data_dir = node_dir / "data"
            data_dir.mkdir(parents=True, exist_ok=True)
            cert_path = node_dir / f"node{node_number}.oucert"
            ca_key_path = user_ca_paths.get(node_number, self.admin_ca_path)
            run_command(
                [
                    "go",
                    "run",
                    "cmd/certgen/main.go",
                    "sign-node",
                    "-ca-key",
                    str(ca_key_path),
                    "-out",
                    str(cert_path),
                ],
                cwd=self.repo_root,
            )
            self.nodes.append(
                NodeRuntime(
                    number=node_number,
                    data_dir=data_dir,
                    cert_path=cert_path,
                    use_color=self.use_color,
                )
            )

    def start_cluster(self) -> None:
        """Start the three-node cluster and wait for carrier mesh."""
        if len(self.nodes) != 3:
            raise RuntimeError("expected three provisioned nodes")

        log("Starting node 1")
        node1 = self.nodes[0]
        node1.start(self.repo_root)
        node1.wait_until_started()

        bootstrap = [node1.listen_address]
        assert bootstrap[0] is not None

        for node in self.nodes[1:]:
            node.bootstrap_addresses = [bootstrap[0]]
            log(f"Starting node {node.number} with bootstrap {bootstrap[0]}")
            node.start(self.repo_root)
            node.wait_until_started()

        self._wait_for_mesh()

    def _wait_for_mesh(self) -> None:
        """Poll peers via REPL until carrier connects all nodes."""
        expected_peers = 2
        deadline = time.monotonic() + MESH_TIMEOUT
        while time.monotonic() < deadline:
            for node in self.nodes:
                if node.is_running():
                    node.send("peers")
            time.sleep(1.0)
            all_ready = True
            for node in self.nodes:
                with node.condition:
                    if len(node.known_peer_ids) < expected_peers:
                        all_ready = False
                        break
            if all_ready:
                log("All nodes connected via carrier mesh")
                return
        details = []
        for node in self.nodes:
            with node.condition:
                count = len(node.known_peer_ids)
            details.append(
                f"node {node.number}: {count}/{expected_peers} peers"
                f"\nlast output:\n{node.tail_output()}"
            )
        raise RuntimeError(
            f"mesh did not form within {MESH_TIMEOUT:.0f}s:\n"
            + "\n".join(details)
        )

    def run_hello(self) -> None:
        """Broadcast one hello from each node and verify cross-node delivery."""
        expected_texts = {
            1: "Hello from Node 1",
            2: "Hello from Node 2",
            3: "Hello from Node 3",
        }

        for node in self.nodes:
            text = expected_texts[node.number]
            prior_line_count = len(node.lines)
            log(f"Broadcasting from node {node.number}: {text}")
            node.send(f"hello {text}")
            node.wait_for(
                lambda current, offset=prior_line_count: any(
                    line == "broadcast delivered to 2 peers, 0 failed"
                    for line in current.lines[offset:]
                ),
                timeout=MESSAGE_TIMEOUT,
                description="broadcast result",
            )

        deadline = time.monotonic() + MESSAGE_TIMEOUT
        while time.monotonic() < deadline:
            if self._all_expected_messages_received(expected_texts):
                break
            time.sleep(POLL_INTERVAL)
        else:
            self._raise_missing_messages(expected_texts)

        log("Hello scenario passed")

    def _all_expected_messages_received(self, expected_texts: dict[int, str]) -> bool:
        """Return True once each node has seen both peers' hello messages."""
        for node in self.nodes:
            seen = {message.text for message in node.received_messages}
            required = {
                text
                for sender, text in expected_texts.items()
                if sender != node.number
            }
            if not required.issubset(seen):
                return False
        return True

    def _raise_missing_messages(self, expected_texts: dict[int, str]) -> None:
        """Raise a detailed error describing any messages that never arrived."""
        details: list[str] = []
        for node in self.nodes:
            seen = {message.text for message in node.received_messages}
            required = {
                text
                for sender, text in expected_texts.items()
                if sender != node.number
            }
            missing = sorted(required - seen)
            if missing:
                details.append(
                    f"node {node.number} missing messages: {', '.join(missing)}\n"
                    f"last output:\n{node.tail_output()}"
                )
        raise RuntimeError("\n\n".join(details))


def run_hello_scenario(args: argparse.Namespace) -> int:
    """Execute the default hello scenario from provision to teardown."""
    repo_root = Path(__file__).resolve().parent
    harness = ClusterHarness(
        repo_root=repo_root,
        keep_temp=args.keep_temp,
        color_mode=args.color,
    )

    timed_out = threading.Event()

    if args.max_time > 0:

        def _watchdog() -> None:
            if timed_out.wait(timeout=args.max_time):
                return
            log(
                f"Maximum test time of {args.max_time:.0f}s exceeded, "
                "shutting down all instances"
            )
            harness.cleanup()
            os._exit(1)

        watchdog_thread = threading.Thread(
            target=_watchdog, daemon=True
        )
        watchdog_thread.start()

    try:
        harness.provision_certificates()
        harness.start_cluster()
        harness.run_hello()
        harness.shutdown_cluster()
        log("All node instances stopped")
        return 0
    finally:
        timed_out.set()
        harness.cleanup()


SCENARIOS: dict[str, Callable[[argparse.Namespace], int]] = {
    "hello": run_hello_scenario,
}


def build_parser() -> argparse.ArgumentParser:
    """Create the CLI parser for scenario selection and runtime options."""
    parser = argparse.ArgumentParser(
        description=(
            "Start three OuroborosDB interactive nodes with go run and run "
            "cluster scenarios. Defaults to the hello scenario."
        )
    )
    parser.add_argument(
        "scenario",
        nargs="?",
        default="hello",
        choices=sorted(SCENARIOS),
        help="scenario to run",
    )
    parser.add_argument(
        "--keep-temp",
        action="store_true",
        help="keep generated certificates and data directories after exit",
    )
    parser.add_argument(
        "--max-time",
        type=float,
        default=MAX_TEST_TIME,
        metavar="SECONDS",
        help=(
            "abort the test if it exceeds this wall-clock duration "
            "(default: %(default)g s, use 0 to disable)"
        ),
    )
    parser.add_argument(
        "--color",
        choices=["auto", "always", "never"],
        default="auto",
        help="colorize node prefixes when printing live output",
    )
    return parser


def main() -> int:
    """Parse CLI arguments and dispatch to the selected scenario."""
    parser = build_parser()
    args = parser.parse_args()
    return SCENARIOS[args.scenario](args)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        log("Interrupted")
        raise SystemExit(130)
#!/usr/bin/env python3
"""
Run gossip network experiments for N in {10, 20, 50}, 5 seeds per N.
Vary TTL, fanout, and neighbors policy. Compute convergence time (95%) and message overhead.
Output CSV and plots (N vs convergence time, N vs overhead).
"""

import argparse
import json
import math
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

try:
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    HAS_MATPLOTLIB = True
except ImportError:
    HAS_MATPLOTLIB = False

# Defaults
PROJECT_ROOT = Path(__file__).resolve().parent.parent
BINARY_NAME = "gogossip"
STABILIZE_SEC = 20
CONVERGENCE_WAIT_SEC = 35
NODES_LIST = [10, 20, 50]
NUM_SEEDS = 5
TTL_VALUES = [5, 10, 20]
FANOUT_VALUES = [2, 3, 5]
POLICY_VALUES = ["first", "random"]
COVERAGE_FRAC = 0.95


def build_binary(project_root: Path) -> Path:
    """Build the Go binary; return path to it."""
    binary = project_root / BINARY_NAME
    cmd = ["go", "build", "-o", str(binary), "./src/cmd/..."]
    subprocess.run(cmd, cwd=project_root, check=True)
    return binary


def run_single_experiment(
    binary: Path,
    project_root: Path,
    n_nodes: int,
    run_id: int,
    ttl: int,
    fanout: int,
    policy: str,
    log_dir: Path,
    peer_limit: int = 100,
) -> tuple[float | None, int | None]:
    """
    Start N nodes, inject one gossip from node 0, wait, then parse logs.
    Returns (convergence_ms, overhead) or (None, None) on failure.
    """
    base_port = 8000
    processes = []
    log_files = []

    try:
        for i in range(n_nodes):
            port = base_port + i
            log_path = log_dir / f"node_{i}.log"
            log_files.append(log_path)
            log_path.write_text("")  # clear

            args = [
                str(binary),
                "-port", str(port),
                "-fanout", str(fanout),
                "-ttl", str(ttl),
                "-peer-limit", str(peer_limit),
                "-experiment-log", str(log_path),
                "-neighbors-policy", policy,
                "-seed", str(12345 + run_id * 1000 + i),
            ]
            if i > 0:
                args.extend(["-bootstrap", f"127.0.0.1:{base_port}"])

            proc = subprocess.Popen(
                args,
                cwd=project_root,
                stdin=subprocess.PIPE,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            processes.append(proc)

        time.sleep(STABILIZE_SEC)

        # Inject one message from node 0 (seed)
        try:
            processes[0].stdin.write(b"__experiment_trigger__\n")
            processes[0].stdin.flush()
        except Exception:
            pass

        time.sleep(CONVERGENCE_WAIT_SEC)

    finally:
        for p in processes:
            try:
                p.terminate()
                p.wait(timeout=5)
            except Exception:
                try:
                    p.kill()
                except Exception:
                    pass

    # Parse logs
    return parse_experiment_logs(log_files, n_nodes)


def parse_experiment_logs(log_files: list[Path], n_nodes: int) -> tuple[float | None, int | None]:
    """
    From experiment log files, get t0 (publish time), recv times per node, and msg_sent counts in [t0, t_95].
    Returns (convergence_ms, overhead).
    """
    # Collect events from all files
    publish_events = []
    recv_events = []
    sent_events = []

    for path in log_files:
        if not path.exists():
            continue
        for line in path.read_text().strip().splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            ev = obj.get("event")
            if ev == "gossip_publish":
                publish_events.append(obj)
            elif ev == "gossip_recv":
                recv_events.append(obj)
            elif ev == "msg_sent":
                sent_events.append(obj)

    if not publish_events:
        return None, None
    # Use first publish as the test message
    pub = publish_events[0]
    t0 = pub.get("ts_ms")
    origin_id = pub.get("origin_id")
    origin_ts = pub.get("origin_ts")
    if t0 is None or origin_id is None or origin_ts is None:
        return None, None

    # Recv times for this message (by origin_id + origin_ts)
    recv_times = []
    for r in recv_events:
        if r.get("origin_id") == origin_id and r.get("origin_ts") == origin_ts:
            recv_times.append(r.get("recv_ms"))
    recv_times = [t for t in recv_times if t is not None]
    if not recv_times:
        return None, None

    # 95% convergence: time by which 95% of nodes have received
    k = max(1, math.ceil(COVERAGE_FRAC * n_nodes))
    recv_times_sorted = sorted(recv_times)
    if k > len(recv_times_sorted):
        t_95 = recv_times_sorted[-1]
    else:
        t_95 = recv_times_sorted[k - 1]

    convergence_ms = float(t_95 - t0)

    # Overhead: all msg_sent in [t0, t_95]
    overhead = sum(1 for s in sent_events if (s.get("ts_ms") or 0) >= t0 and (s.get("ts_ms") or 0) <= t_95)

    return convergence_ms, overhead


def run_all_experiments(
    project_root: Path,
    out_dir: Path,
    nodes_list: list[int],
    num_seeds: int,
    ttl_values: list[int],
    fanout_values: list[int],
    policy_values: list[str],
) -> list[dict]:
    """Run experiments and return list of result rows."""
    binary = build_binary(project_root)
    results = []

    total = (
        len(nodes_list) * num_seeds * len(ttl_values) * len(fanout_values) * len(policy_values)
    )
    run_idx = 0

    for n_nodes in nodes_list:
        for run_id in range(num_seeds):
            for ttl in ttl_values:
                for fanout in fanout_values:
                    for policy in policy_values:
                        run_idx += 1
                        print(f"[{run_idx}/{total}] N={n_nodes} seed={run_id} ttl={ttl} fanout={fanout} policy={policy} ...")
                        with tempfile.TemporaryDirectory(prefix="gossip_exp_") as log_dir:
                            log_path = Path(log_dir)
                            conv_ms, overhead = run_single_experiment(
                                binary, project_root, n_nodes, run_id, ttl, fanout, policy, log_path
                            )
                        if conv_ms is not None and overhead is not None:
                            results.append({
                                "N": n_nodes,
                                "run": run_id,
                                "ttl": ttl,
                                "fanout": fanout,
                                "policy": policy,
                                "convergence_ms": conv_ms,
                                "overhead": overhead,
                            })
                            print(f"  -> convergence={conv_ms:.0f} ms, overhead={overhead}")
                        else:
                            print("  -> (parse failed, skipping)")

    return results


def write_csv(results: list[dict], path: Path) -> None:
    if not results:
        return
    keys = list(results[0].keys())
    lines = [",".join(keys)]
    for r in results:
        lines.append(",".join(str(r[k]) for k in keys))
    path.write_text("\n".join(lines) + "\n")


def plot_results(results: list[dict], out_dir: Path) -> None:
    if not HAS_MATPLOTLIB or not results:
        return

    import numpy as np

    # Aggregate: for each (N, ttl, fanout, policy) take mean over runs
    from collections import defaultdict
    agg = defaultdict(list)
    for r in results:
        key = (r["N"], r["ttl"], r["fanout"], r["policy"])
        agg[key].append(r)

    nodes_list = sorted({r["N"] for r in results})
    ttl_vals = sorted({r["ttl"] for r in results})
    fanout_vals = sorted({r["fanout"] for r in results})
    policy_vals = sorted({r["policy"] for r in results})

    def mean_convergence(key):
        return np.mean([x["convergence_ms"] for x in agg[key]])

    def mean_overhead(key):
        return np.mean([x["overhead"] for x in agg[key]])

    # Plot 1: Convergence time vs N, one curve per (ttl, fanout, policy) or simplified
    fig, ax = plt.subplots(figsize=(8, 5))
    for ttl in ttl_vals:
        for fanout in fanout_vals:
            for policy in policy_vals:
                label = f"ttl={ttl} fanout={fanout} {policy}"
                x, y = [], []
                for N in nodes_list:
                    key = (N, ttl, fanout, policy)
                    if key in agg and agg[key]:
                        x.append(N)
                        y.append(mean_convergence(key))
                if x:
                    ax.plot(x, y, "o-", label=label)
    ax.set_xlabel("N (number of nodes)")
    ax.set_ylabel("Convergence time (ms)")
    ax.set_title("95% Convergence time vs N")
    ax.legend(bbox_to_anchor=(1.02, 1), loc="upper left", fontsize=7)
    ax.grid(True, alpha=0.3)
    plt.tight_layout()
    fig.savefig(out_dir / "convergence_vs_N.png", dpi=120, bbox_inches="tight")
    plt.close()

    # Plot 2: Overhead vs N
    fig, ax = plt.subplots(figsize=(8, 5))
    for ttl in ttl_vals:
        for fanout in fanout_vals:
            for policy in policy_vals:
                label = f"ttl={ttl} fanout={fanout} {policy}"
                x, y = [], []
                for N in nodes_list:
                    key = (N, ttl, fanout, policy)
                    if key in agg and agg[key]:
                        x.append(N)
                        y.append(mean_overhead(key))
                if x:
                    ax.plot(x, y, "s-", label=label)
    ax.set_xlabel("N (number of nodes)")
    ax.set_ylabel("Message overhead (count)")
    ax.set_title("Message overhead (until 95% coverage) vs N")
    ax.legend(bbox_to_anchor=(1.02, 1), loc="upper left", fontsize=7)
    ax.grid(True, alpha=0.3)
    plt.tight_layout()
    fig.savefig(out_dir / "overhead_vs_N.png", dpi=120, bbox_inches="tight")
    plt.close()

    # Simpler plots: fix fanout and policy, vary TTL
    default_fanout = fanout_vals[0] if fanout_vals else 3
    default_policy = policy_vals[0] if policy_vals else "first"
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4))
    for ttl in ttl_vals:
        label = f"TTL={ttl}"
        x, y1, y2 = [], [], []
        for N in nodes_list:
            key = (N, ttl, default_fanout, default_policy)
            if key in agg and agg[key]:
                x.append(N)
                y1.append(mean_convergence(key))
                y2.append(mean_overhead(key))
        if x:
            ax1.plot(x, y1, "o-", label=label)
            ax2.plot(x, y2, "s-", label=label)
    ax1.set_xlabel("N")
    ax1.set_ylabel("Convergence (ms)")
    ax1.set_title("Convergence vs N (fanout=3, first)")
    ax1.legend()
    ax1.grid(True, alpha=0.3)
    ax2.set_xlabel("N")
    ax2.set_ylabel("Overhead")
    ax2.set_title("Overhead vs N (fanout=3, first)")
    ax2.legend()
    ax2.grid(True, alpha=0.3)
    plt.tight_layout()
    fig.savefig(out_dir / "ttl_effect.png", dpi=120, bbox_inches="tight")
    plt.close()


def main():
    parser = argparse.ArgumentParser(description="Run gossip experiments")
    parser.add_argument("--project-root", type=Path, default=PROJECT_ROOT, help="Go project root")
    parser.add_argument("--out-dir", type=Path, default=None, help="Output directory (default: project_root/experiment_results)")
    parser.add_argument("--N", type=int, nargs="+", default=NODES_LIST, help="Node counts")
    parser.add_argument("--seeds", type=int, default=NUM_SEEDS, help="Number of runs per config")
    parser.add_argument("--ttl", type=int, nargs="+", default=TTL_VALUES, help="TTL values")
    parser.add_argument("--fanout", type=int, nargs="+", default=FANOUT_VALUES, help="Fanout values")
    parser.add_argument("--policy", type=str, nargs="+", default=POLICY_VALUES, help="Neighbors policy")
    parser.add_argument("--no-plot", action="store_true", help="Skip plotting")
    args = parser.parse_args()

    out_dir = args.out_dir or args.project_root / "experiment_results"
    out_dir.mkdir(parents=True, exist_ok=True)

    results = run_all_experiments(
        args.project_root,
        out_dir,
        nodes_list=args.N,
        num_seeds=args.seeds,
        ttl_values=args.ttl,
        fanout_values=args.fanout,
        policy_values=args.policy,
    )

    csv_path = out_dir / "results.csv"
    write_csv(results, csv_path)
    print(f"Wrote {csv_path} ({len(results)} rows)")

    if not args.no_plot and results:
        plot_results(results, out_dir)
        print(f"Plots saved in {out_dir}")
    elif not HAS_MATPLOTLIB and not args.no_plot:
        print("Install matplotlib for plots: pip install matplotlib", file=sys.stderr)


if __name__ == "__main__":
    main()

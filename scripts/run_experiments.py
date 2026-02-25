#!/usr/bin/env python3
"""
Run gossip experiments in four parts:

  1. N×seed: Effect of N and seeds. Normal config. Two versions: push-only and push-pull hybrid.
  2. TTL:    Only change TTL; fix others; push-only.
  3. Policy: Only change policy; fix others; push-only.
  4. Fanout: Only change fanout; fix others; push-only.

Normal config: ttl=10, fanout=3, policy=first.
"""

import argparse
import json
import math
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

PROJECT_ROOT = Path(__file__).resolve().parent.parent
BINARY_NAME = "gogossip"
STABILIZE_SEC = 20
CONVERGENCE_WAIT_SEC = 35
NODES_LIST = [10, 20, 50]
NUM_SEEDS = 5
COVERAGE_FRAC = 0.95
IHAVE_MAX_IDS = 32
# Pull interval for hybrid (near ping so convergence isn't delayed by next IHAVE)
PULL_MS_HYBRID = 1000

# Normal config (used when not varying that parameter)
TTL_NORMAL = 10
FANOUT_NORMAL = 3
POLICY_NORMAL = "first"

TTL_VALUES = [5, 10, 20]
FANOUT_VALUES = [2, 3, 5]
POLICY_VALUES = ["first", "random"]


def build_binary(project_root: Path) -> Path:
    binary = project_root / BINARY_NAME
    subprocess.run(["go", "build", "-o", str(binary), "./src/cmd/..."], cwd=project_root, check=True)
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
    pull_interval_ms: int = 0,
) -> tuple[float | None, int | None]:
    base_port = 8000
    processes = []
    log_files = []
    try:
        for i in range(n_nodes):
            port = base_port + i
            log_path = log_dir / f"node_{i}.log"
            log_files.append(log_path)
            log_path.write_text("")
            args = [
                str(binary),
                "-port", str(port),
                "-fanout", str(fanout),
                "-ttl", str(ttl),
                "-peer-limit", "100",
                "-experiment-log", str(log_path),
                "-neighbors-policy", policy,
                "-seed", str(12345 + run_id * 1000 + i),
            ]
            if pull_interval_ms > 0:
                args.extend(["-pull-interval", str(pull_interval_ms), "-ihave-max-ids", str(IHAVE_MAX_IDS)])
            if i > 0:
                args.extend(["-bootstrap", f"127.0.0.1:{base_port}"])
            proc = subprocess.Popen(
                args, cwd=project_root, stdin=subprocess.PIPE,
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            )
            processes.append(proc)
        time.sleep(STABILIZE_SEC)
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
    return parse_experiment_logs(log_files, n_nodes)


def parse_experiment_logs(log_files: list[Path], n_nodes: int) -> tuple[float | None, int | None]:
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
    pub = publish_events[0]
    t0 = pub.get("ts_ms")
    origin_id = pub.get("origin_id")
    origin_ts = pub.get("origin_ts")
    if t0 is None or origin_id is None or origin_ts is None:
        return None, None
    recv_times = []
    for r in recv_events:
        if r.get("origin_id") == origin_id and r.get("origin_ts") == origin_ts:
            recv_times.append(r.get("recv_ms"))
    recv_times = [t for t in recv_times if t is not None]
    if not recv_times:
        return None, None
    k = max(1, math.ceil(COVERAGE_FRAC * n_nodes))
    recv_times_sorted = sorted(recv_times)
    t_95 = recv_times_sorted[k - 1] if k <= len(recv_times_sorted) else recv_times_sorted[-1]
    convergence_ms = float(t_95 - t0)
    overhead = sum(1 for s in sent_events if (s.get("ts_ms") or 0) >= t0 and (s.get("ts_ms") or 0) <= t_95)
    return convergence_ms, overhead


def run_n_seed_test(
    binary: Path,
    project_root: Path,
    nodes_list: list[int],
    num_seeds: int,
) -> list[dict]:
    """Part 1: N×seed with normal config. Two versions: push-only and push-pull hybrid."""
    results = []
    total = len(nodes_list) * num_seeds * 2  # push + hybrid
    run_idx = 0
    for n_nodes in nodes_list:
        for run_id in range(num_seeds):
            for pull_ms in [0, PULL_MS_HYBRID]:
                run_idx += 1
                mode = "hybrid" if pull_ms else "push_only"
                print(f"[N×seed {run_idx}/{total}] N={n_nodes} seed={run_id} {mode} ...")
                with tempfile.TemporaryDirectory(prefix="gossip_") as log_dir:
                    conv_ms, overhead = run_single_experiment(
                        binary, project_root, n_nodes, run_id,
                        ttl=TTL_NORMAL, fanout=FANOUT_NORMAL, policy=POLICY_NORMAL,
                        log_dir=Path(log_dir), pull_interval_ms=pull_ms,
                    )
                if conv_ms is not None and overhead is not None:
                    results.append({
                        "test": "n_seed",
                        "N": n_nodes,
                        "run": run_id,
                        "vary": f"N={n_nodes}_seed={run_id}",
                        "ttl": TTL_NORMAL,
                        "fanout": FANOUT_NORMAL,
                        "policy": POLICY_NORMAL,
                        "pull_interval_ms": pull_ms,
                        "convergence_ms": conv_ms,
                        "overhead": overhead,
                    })
                    print(f"  -> convergence={conv_ms:.0f} ms, overhead={overhead}")
                else:
                    print("  -> (parse failed)")
    return results


def run_ttl_test(
    binary: Path,
    project_root: Path,
    nodes_list: list[int],
    num_seeds: int,
) -> list[dict]:
    """Part 2: Vary TTL only; normal fanout, policy. Push-only. N×seed combinations."""
    results = []
    total = len(nodes_list) * num_seeds * len(TTL_VALUES)
    run_idx = 0
    for n_nodes in nodes_list:
        for run_id in range(num_seeds):
            for ttl in TTL_VALUES:
                run_idx += 1
                print(f"[TTL {run_idx}/{total}] N={n_nodes} seed={run_id} ttl={ttl} push_only ...")
                with tempfile.TemporaryDirectory(prefix="gossip_") as log_dir:
                    conv_ms, overhead = run_single_experiment(
                        binary, project_root, n_nodes, run_id,
                        ttl=ttl, fanout=FANOUT_NORMAL, policy=POLICY_NORMAL,
                        log_dir=Path(log_dir), pull_interval_ms=0,
                    )
                if conv_ms is not None and overhead is not None:
                    results.append({
                        "test": "ttl",
                        "N": n_nodes,
                        "run": run_id,
                        "vary": f"ttl={ttl}",
                        "ttl": ttl,
                        "fanout": FANOUT_NORMAL,
                        "policy": POLICY_NORMAL,
                        "pull_interval_ms": 0,
                        "convergence_ms": conv_ms,
                        "overhead": overhead,
                    })
                    print(f"  -> convergence={conv_ms:.0f} ms, overhead={overhead}")
                else:
                    print("  -> (parse failed)")
    return results


def run_policy_test(
    binary: Path,
    project_root: Path,
    nodes_list: list[int],
    num_seeds: int,
) -> list[dict]:
    """Part 3: Vary policy only; normal ttl, fanout. Push-only. N×seed combinations."""
    results = []
    total = len(nodes_list) * num_seeds * len(POLICY_VALUES)
    run_idx = 0
    for n_nodes in nodes_list:
        for run_id in range(num_seeds):
            for policy in POLICY_VALUES:
                run_idx += 1
                print(f"[Policy {run_idx}/{total}] N={n_nodes} seed={run_id} policy={policy} push_only ...")
                with tempfile.TemporaryDirectory(prefix="gossip_") as log_dir:
                    conv_ms, overhead = run_single_experiment(
                        binary, project_root, n_nodes, run_id,
                        ttl=TTL_NORMAL, fanout=FANOUT_NORMAL, policy=policy,
                        log_dir=Path(log_dir), pull_interval_ms=0,
                    )
                if conv_ms is not None and overhead is not None:
                    results.append({
                        "test": "policy",
                        "N": n_nodes,
                        "run": run_id,
                        "vary": f"policy={policy}",
                        "ttl": TTL_NORMAL,
                        "fanout": FANOUT_NORMAL,
                        "policy": policy,
                        "pull_interval_ms": 0,
                        "convergence_ms": conv_ms,
                        "overhead": overhead,
                    })
                    print(f"  -> convergence={conv_ms:.0f} ms, overhead={overhead}")
                else:
                    print("  -> (parse failed)")
    return results


def run_fanout_test(
    binary: Path,
    project_root: Path,
    nodes_list: list[int],
    num_seeds: int,
) -> list[dict]:
    """Part 4: Vary fanout only; normal ttl, policy. Push-only. N×seed combinations."""
    results = []
    total = len(nodes_list) * num_seeds * len(FANOUT_VALUES)
    run_idx = 0
    for n_nodes in nodes_list:
        for run_id in range(num_seeds):
            for fanout in FANOUT_VALUES:
                run_idx += 1
                print(f"[Fanout {run_idx}/{total}] N={n_nodes} seed={run_id} fanout={fanout} push_only ...")
                with tempfile.TemporaryDirectory(prefix="gossip_") as log_dir:
                    conv_ms, overhead = run_single_experiment(
                        binary, project_root, n_nodes, run_id,
                        ttl=TTL_NORMAL, fanout=fanout, policy=POLICY_NORMAL,
                        log_dir=Path(log_dir), pull_interval_ms=0,
                    )
                if conv_ms is not None and overhead is not None:
                    results.append({
                        "test": "fanout",
                        "N": n_nodes,
                        "run": run_id,
                        "vary": f"fanout={fanout}",
                        "ttl": TTL_NORMAL,
                        "fanout": fanout,
                        "policy": POLICY_NORMAL,
                        "pull_interval_ms": 0,
                        "convergence_ms": conv_ms,
                        "overhead": overhead,
                    })
                    print(f"  -> convergence={conv_ms:.0f} ms, overhead={overhead}")
                else:
                    print("  -> (parse failed)")
    return results


def write_csv(results: list[dict], path: Path) -> None:
    if not results:
        return
    keys = list(results[0].keys())
    lines = [",".join(keys)]
    for r in results:
        lines.append(",".join(str(r[k]) for k in keys))
    path.write_text("\n".join(lines) + "\n")


def plot_n_seed_results(results: list[dict], out_dir: Path) -> None:
    """Part 1: Convergence and overhead vs N; two curves = push_only vs hybrid (mean over seeds)."""
    if not HAS_MATPLOTLIB or not results:
        return
    from collections import defaultdict
    import numpy as np
    agg = defaultdict(list)
    for r in results:
        key = (r["N"], r["pull_interval_ms"])
        agg[key].append(r)
    nodes_list = sorted({r["N"] for r in results})
    pull_vals = sorted({r["pull_interval_ms"] for r in results})

    def mean_conv(key):
        return np.mean([x["convergence_ms"] for x in agg[key]])

    def mean_overhead(key):
        return np.mean([x["overhead"] for x in agg[key]])

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4))
    for pull_ms in pull_vals:
        label = "push-pull hybrid" if pull_ms else "push only"
        x, y1, y2 = [], [], []
        for N in nodes_list:
            key = (N, pull_ms)
            if key in agg and agg[key]:
                x.append(N)
                y1.append(mean_conv(key))
                y2.append(mean_overhead(key))
        if x:
            ax1.plot(x, y1, "o-", label=label)
            ax2.plot(x, y2, "s-", label=label)
    ax1.set_xlabel("N")
    ax1.set_ylabel("Convergence (ms)")
    ax1.set_title("Part 1: N×seed — Convergence vs N (mean over seeds)")
    ax1.legend()
    ax1.grid(True, alpha=0.3)
    ax2.set_xlabel("N")
    ax2.set_ylabel("Overhead")
    ax2.set_title("Part 1: N×seed — Overhead vs N (mean over seeds)")
    ax2.legend()
    ax2.grid(True, alpha=0.3)
    plt.tight_layout()
    fig.savefig(out_dir / "plot_n_seed.png", dpi=120, bbox_inches="tight")
    plt.close()


def plot_focused_results(results: list[dict], out_dir: Path, test_name: str) -> None:
    """Plot convergence and overhead vs N for one test; curves = vary (push-only for ttl/policy/fanout)."""
    if not HAS_MATPLOTLIB or not results:
        return
    from collections import defaultdict
    import numpy as np
    agg = defaultdict(list)
    for r in results:
        key = (r["N"], r["vary"], r["pull_interval_ms"])
        agg[key].append(r)
    nodes_list = sorted({r["N"] for r in results})
    vary_vals = sorted({r["vary"] for r in results})
    pull_vals = sorted({r["pull_interval_ms"] for r in results})

    def mean_conv(key):
        return np.mean([x["convergence_ms"] for x in agg[key]])

    def mean_overhead(key):
        return np.mean([x["overhead"] for x in agg[key]])

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4))
    for vary in vary_vals:
        for pull_ms in pull_vals:
            label = f"{vary}" + (" hybrid" if pull_ms else " push_only")
            x, y1, y2 = [], [], []
            for N in nodes_list:
                key = (N, vary, pull_ms)
                if key in agg and agg[key]:
                    x.append(N)
                    y1.append(mean_conv(key))
                    y2.append(mean_overhead(key))
            if x:
                ax1.plot(x, y1, "o-", label=label)
                ax2.plot(x, y2, "s-", label=label)
    ax1.set_xlabel("N")
    ax1.set_ylabel("Convergence (ms)")
    ax1.set_title(f"Test: {test_name} — Convergence vs N (push only)")
    ax1.legend(fontsize=8)
    ax1.grid(True, alpha=0.3)
    ax2.set_xlabel("N")
    ax2.set_ylabel("Overhead")
    ax2.set_title(f"Test: {test_name} — Overhead vs N (push only)")
    ax2.legend(fontsize=8)
    ax2.grid(True, alpha=0.3)
    plt.tight_layout()
    fig.savefig(out_dir / f"plot_{test_name}.png", dpi=120, bbox_inches="tight")
    plt.close()


def main():
    parser = argparse.ArgumentParser(
        description="Run experiments: part 1 N×seed (push+hybrid), parts 2–4 ttl/policy/fanout (push only)."
    )
    parser.add_argument("--project-root", type=Path, default=PROJECT_ROOT)
    parser.add_argument("--out-dir", type=Path, default=None)
    parser.add_argument("--test", choices=["n_seed", "ttl", "policy", "fanout", "all"], default="all",
                        help="Part to run: n_seed, ttl, policy, fanout, or all")
    parser.add_argument("--N", type=int, nargs="+", default=NODES_LIST)
    parser.add_argument("--seeds", type=int, default=NUM_SEEDS)
    parser.add_argument("--no-plot", action="store_true")
    args = parser.parse_args()

    out_dir = args.out_dir or args.project_root / "experiment_results"
    out_dir.mkdir(parents=True, exist_ok=True)

    binary = build_binary(args.project_root)
    nodes_list = args.N
    num_seeds = args.seeds
    all_results = []

    if args.test in ("n_seed", "all"):
        results_n_seed = run_n_seed_test(binary, args.project_root, nodes_list, num_seeds)
        write_csv(results_n_seed, out_dir / "results_n_seed.csv")
        print(f"Wrote {out_dir / 'results_n_seed.csv'} ({len(results_n_seed)} rows)")
        all_results.extend(results_n_seed)
        if not args.no_plot and results_n_seed:
            plot_n_seed_results(results_n_seed, out_dir)

    if args.test in ("ttl", "all"):
        results_ttl = run_ttl_test(binary, args.project_root, nodes_list, num_seeds)
        write_csv(results_ttl, out_dir / "results_ttl.csv")
        print(f"Wrote {out_dir / 'results_ttl.csv'} ({len(results_ttl)} rows)")
        all_results.extend(results_ttl)
        if not args.no_plot and results_ttl:
            plot_focused_results(results_ttl, out_dir, "ttl")

    if args.test in ("policy", "all"):
        results_policy = run_policy_test(binary, args.project_root, nodes_list, num_seeds)
        write_csv(results_policy, out_dir / "results_policy.csv")
        print(f"Wrote {out_dir / 'results_policy.csv'} ({len(results_policy)} rows)")
        all_results.extend(results_policy)
        if not args.no_plot and results_policy:
            plot_focused_results(results_policy, out_dir, "policy")

    if args.test in ("fanout", "all"):
        results_fanout = run_fanout_test(binary, args.project_root, nodes_list, num_seeds)
        write_csv(results_fanout, out_dir / "results_fanout.csv")
        print(f"Wrote {out_dir / 'results_fanout.csv'} ({len(results_fanout)} rows)")
        all_results.extend(results_fanout)
        if not args.no_plot and results_fanout:
            plot_focused_results(results_fanout, out_dir, "fanout")

    if all_results:
        write_csv(all_results, out_dir / "results.csv")
        print(f"Wrote {out_dir / 'results.csv'} (combined {len(all_results)} rows)")
    if not HAS_MATPLOTLIB and not args.no_plot:
        print("Install matplotlib for plots: pip install matplotlib", file=sys.stderr)


if __name__ == "__main__":
    main()

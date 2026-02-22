# Gossip experiment script

Run automated experiments for the gossip protocol: measure **95% convergence time** and **message overhead** for different N, TTL, fanout, and neighbors policy.

## Metrics

- **Convergence time**: From message creation (t0) until the time when 95% of nodes have received that message.
- **Message overhead**: Total number of messages sent (gossip + HELLO, GET_PEERS, PEERS_LIST, PING, PONG) from t0 until 95% coverage.

## Prerequisites

- Go (to build the node binary)
- Python 3.9+
- For plots: `pip install -r scripts/requirements.txt` (matplotlib, numpy)

## Usage

From the **project root** (GoGossip/):

```bash
# Default: N=10,20,50 × 5 seeds × TTL=5,10,20 × fanout=2,3,5 × policy=first,random
python3 scripts/run_experiments.py

# Custom node counts and fewer runs (faster)
python3 scripts/run_experiments.py --N 10 20 --seeds 2 --ttl 10 --fanout 3 --policy first

# Output to a specific directory
python3 scripts/run_experiments.py --out-dir ./my_results

# Skip plotting (only CSV)
python3 scripts/run_experiments.py --no-plot
```

## Output

- `experiment_results/results.csv`: columns N, run, ttl, fanout, policy, convergence_ms, overhead
- `experiment_results/convergence_vs_N.png`: N on x-axis, convergence time (ms) on y-axis, one curve per (ttl, fanout, policy)
- `experiment_results/overhead_vs_N.png`: N on x-axis, message count on y-axis
- `experiment_results/ttl_effect.png`: two panels (convergence and overhead vs N) for different TTL values

## Node flags used by the script

- `-experiment-log <path>`: append JSON metric lines (gossip_recv, gossip_publish, msg_sent)
- `-neighbors-policy first|random`: how to choose peers when forwarding (first in list vs random)

# Gossip experiment script

Four parts:

1. **N×seed** – Effect of N and seeds. Normal config. **Two versions:** push-only and push-pull hybrid.  
   Runs: N × seeds × 2 (e.g. 3×5×2 = 30).

2. **TTL** – Only change TTL; fix fanout=3, policy=first. **Push-only.**  
   Runs: N × seeds × 3 ttls (e.g. 45).

3. **Policy** – Only change policy; fix ttl=10, fanout=3. **Push-only.**  
   Runs: N × seeds × 2 policies (e.g. 30).

4. **Fanout** – Only change fanout; fix ttl=10, policy=first. **Push-only.**  
   Runs: N × seeds × 3 fanouts (e.g. 45).

**Normal config:** `ttl=10`, `fanout=3`, `policy=first`.

## Usage

From the **project root** (GoGossip/):

```bash
# Run all four parts
python3 scripts/run_experiments.py

# Run only one part
python3 scripts/run_experiments.py --test n_seed
python3 scripts/run_experiments.py --test ttl
python3 scripts/run_experiments.py --test policy
python3 scripts/run_experiments.py --test fanout

# Fewer N and seeds (faster)
python3 scripts/run_experiments.py --N 10 20 --seeds 2

# Custom output directory
python3 scripts/run_experiments.py --out-dir ./my_results

# Skip plots
python3 scripts/run_experiments.py --no-plot
```

## Output

- **results_n_seed.csv** – Part 1: N×seed, push_only and hybrid (columns: test, N, run, vary, pull_interval_ms, convergence_ms, overhead).
- **results_ttl.csv** – Part 2: TTL only, push only.
- **results_policy.csv** – Part 3: Policy only, push only.
- **results_fanout.csv** – Part 4: Fanout only, push only.
- **results.csv** – Combined (when `--test all`).
- **plot_n_seed.png** – Part 1: convergence and overhead vs N, two curves (push only vs hybrid, mean over seeds).
- **plot_ttl.png**, **plot_policy.png**, **plot_fanout.png** – Parts 2–4: one curve per varied value (push only).

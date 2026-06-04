# GoGossip


GoGossip is a highly configurable, peer-to-peer (P2P) UDP-based gossip protocol written in Go. It enables decentralized message broadcasting and provides an extensible framework for studying fundamental P2P network behaviors such as convergence time and network overhead. 

The project comes complete with both **push** and **pull** dissemination strategies, a **Proof-of-Work (PoW)** sybil defense mechanism for peer discovery, and a robust **experimentation suite** written in Python to evaluate the performance of the protocol under varied topologies and network constraints.

---

## 🚀 Key Features

- **Push-Pull Hybrid Gossip:** Configurable dissemination using active Push (`GOSSIP` messages) and reactive Pull (`IHAVE`, `IWANT` messages) mechanisms.
- **UDP Networking:** Lightweight, connectionless, high-throughput network layer.
- **Proof-of-Work (PoW) Peer Discovery:** Prevents spam and Sybil attacks during peer handshakes by demanding computational effort per `HELLO` message.
- **Configurable Topologies:** Fine-tune `Fanout`, `TTL`, `Peer Limit`, and Fanout policies (`first` vs. `random`).
- **Distributed Caching:** Efficient local bounded caching to serve `IWANT` requests for missed payloads.
- **Comprehensive Benchmarking:** Automated Python experimentation scripts to simulate N-node networks, record network overhead, and plot convergence metrics.
- **Graceful Shutdown & Clean Architecture:** Strongly decoupled internal modules, extensive usage of Go's `context` for lifecycle management, and structured logging using `slog`.

---

## 🏗️ Architecture

- **`node/`**: The core logic of the decentralized node. Handles lifecycle, background routines (ping, prune, pull), and handles all incoming handlers.
- **`network/`**: Standardizes non-blocking UDP listeners and dispatch mechanisms.
- **`cache/`**: Thread-safe caching for `GOSSIP` payloads, allowing responsive `IWANT` fulfillment without keeping data indefinitely.
- **`pow/`**: Contains the Proof of Work hashing mechanisms (Hashcash-style) for node verification.
- **`experiment/`**: Experimentation logs metrics like convergence (time for a message to reach all nodes) and overhead (redundant messages per node) as JSON lines.

---

## 🛠️ Setup & Installation

### Requirements
- **Go 1.24+**
- **Python 3.x** (with `matplotlib` and `pandas` for running experiments and plotting)

### Installation
Clone the repository and download Go dependencies:

```bash
git clone https://github.com/mohammadaminkoohi/GoGossip.git
cd GoGossip
go mod download
```

---

## 🕹️ Usage

### Starting the Seed Node (Bootstrap Node)
Start the very first node of the network. It will act as the bootstrap for subsequent nodes.

```bash
go run src/cmd/main.go --port 8000
```

### Starting a Peer Node
Start a new node and connect it to the bootstrap node:

```bash
go run src/cmd/main.go --port 8001 --bootstrap 127.0.0.1:8000
go run src/cmd/main.go --port 8002 --bootstrap 127.0.0.1:8000
```
*Tip: Type a message into any node's console and press enter to gossip it across the network!*

### CLI Flags

- `--port`: Listening port (default 8000)
- `--bootstrap`: Address of the seed node (IP:PORT)
- `--fanout`: Number of peers to gossip to (default 3)
- `--ttl`: Time-to-Live for messages (default 10)
- `--pull-interval`: IHAVE send interval in ms. Set > 0 for Hybrid push/pull (default 0).
- `--pow-k`: PoW difficulty (leading hex-zeros required in HELLO). Use 0 to disable.
- `--neighbors-policy`: Fanout selection: `first` or `random`.
- `--debug`: Enable verbose debug logging.

---

## 📁 Project Structure

```text
GoGossip/
├── go.mod
├── experiment_results/      # Generated CSV metrics and graphical plots
├── scripts/
│   ├── run_experiments.py   # Main Python automation suite
│   ├── requirements.txt
│   └── README.md
├── src/
│   ├── cmd/
│   │   └── main.go          # CLI Entrypoint
│   └── internal/            # Core library features
│       ├── cache/           # Gossip payload caching
│       ├── message/         # Wire formats & serialization
│       ├── network/         # UDP layer
│       ├── node/            # Protocol & lifecycle orchestration
│       ├── peer/            # Thread-safe peer management
│       ├── pow/             # Proof of Work logic
│       └── seen/            # Thread-safe ID sets (anti-loop)
```


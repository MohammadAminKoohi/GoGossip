package node

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/cache"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/message"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/network"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/peer"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/pow"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/seen"
)

type Config struct {
	Port               int
	Bootstrap          string
	Fanout             int
	TTL                int
	PeerLimit          int
	PingInterval       int
	PeerTimeout        int
	Seed               int64
	ExperimentLogPath  string // if set, append JSON metric lines for experiment script
	NeighborsPolicy    string // "first" (default) or "random" for fanout selection
	PullInterval       int    // ms; if > 0, send IHAVE to neighbors at this interval
	IHaveMaxIds        int    // max message IDs to put in one IHAVE message
	GossipCacheMaxSize int    // max gossip payloads to cache for IWANT responses (0 = default)
	PowK               int    // number of leading hex-zero nibbles required for HELLO PoW (0 = disabled)
}

type Node struct {
	cfg      Config
	selfAddr string
	uuid     uuid.UUID

	peers        *peer.Store
	seen         *seen.Set
	helloReplied *seen.Set
	gossipCache  *cache.GossipCache

	rng      *rand.Rand
	powProof *message.PoWProof

	expLog   *json.Encoder
	expLogMu sync.Mutex

	cancel context.CancelFunc
}

func New(cfg Config) (*Node, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		slog.Error("invalid port", slog.Int("port", cfg.Port))
		return nil, fmt.Errorf("invalid port: %d", cfg.Port)
	}

	n := &Node{
		cfg:          cfg,
		selfAddr:     fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		uuid:         uuid.New(),
		peers:        peer.NewStore(cfg.PeerLimit, cfg.PeerTimeout),
		seen:         seen.NewSet(),
		helloReplied: seen.NewSet(),
		gossipCache:  cache.New(cfg.GossipCacheMaxSize),
	}

	if cfg.ExperimentLogPath != "" {
		f, err := os.OpenFile(cfg.ExperimentLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open experiment log: %w", err)
		}
		enc := json.NewEncoder(f)
		enc.SetEscapeHTML(false)
		n.expLog = enc
	}

	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	n.rng = rand.New(rand.NewSource(seed))

	if cfg.PowK > 0 {
		slog.Info("mining PoW for HELLO", slog.Int("difficulty_k", cfg.PowK))
		nonce, digest := pow.Mine(n.uuid.String(), cfg.PowK)
		n.powProof = &message.PoWProof{
			HashAlg:     pow.HashAlg,
			DifficultyK: cfg.PowK,
			Nonce:       nonce,
			DigestHex:   digest,
		}
		slog.Info("PoW mined", slog.Uint64("nonce", nonce), slog.String("digest", digest))
	}

	slog.Info("node created", slog.String("uuid", n.uuid.String()), slog.String("addr", n.selfAddr))
	return n, nil
}

// Start begins listening for packets, starts background loops, and joins the network via bootstrap.
// The provided ctx controls the lifetime of all background goroutines; cancel it (or call Stop) to shut down.
func (n *Node) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	go func() {
		if err := network.Listen(ctx, n.selfAddr, n.handlePacket); err != nil {
			slog.Error("listener exited", slog.String("error", err.Error()))
		}
	}()
	go n.runPingAndPruneLoop(ctx)
	if n.cfg.PullInterval > 0 {
		go n.runPullLoop(ctx)
	}
	if n.cfg.Bootstrap != "" {
		if err := n.sendHelloToBootstrap(); err != nil {
			return err
		}
		if err := n.sendGetPeersToBootstrap(); err != nil {
			return err
		}
	}
	return nil
}

// Stop cancels the context that drives all background goroutines, shutting the node down cleanly.
func (n *Node) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
}

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/node"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, debug := parseFlags()

	logger := setupLogger(logLevel(debug))
	slog.SetDefault(logger)

	logger.Info("configuration loaded", slog.Any("config", cfg))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	n, err := node.New(cfg)
	if err != nil {
		return fmt.Errorf("init node: %w", err)
	}
	defer n.Stop()

	if cfg.Bootstrap != "" {
		logger.Info("starting as peer", slog.String("bootstrap", cfg.Bootstrap), slog.Int("port", cfg.Port))
	} else {
		logger.Info("starting as seed node", slog.Int("port", cfg.Port))
	}

	if err := n.Start(ctx); err != nil {
		return fmt.Errorf("start node: %w", err)
	}

	logger.Info("node running, type a message and press Enter to gossip it (Ctrl+C to exit)")

	if err := runCLI(ctx, n, logger); err != nil {
		return err
	}

	logger.Info("shutting down")
	return nil
}

func parseFlags() (node.Config, bool) {
	var cfg node.Config
	var debug bool

	flag.BoolVar(&debug, "debug", false, "Enable debug logs (ping/pong detail, etc.)")
	flag.IntVar(&cfg.Port, "port", 8000, "Listening port for this Node")
	flag.StringVar(&cfg.Bootstrap, "bootstrap", "", "Address of the seed Node")
	flag.IntVar(&cfg.Fanout, "fanout", 3, "Fanout value for this Node")
	flag.IntVar(&cfg.TTL, "ttl", 10, "TTL value for this Node")
	flag.IntVar(&cfg.PeerLimit, "peer-limit", 100, "Peer limit for this Node")
	flag.IntVar(&cfg.PingInterval, "ping-interval", 1000, "Ping interval in milliseconds for this Node")
	flag.IntVar(&cfg.PeerTimeout, "peer-timeout", 5000, "Peer timeout in milliseconds for this Node")
	flag.Int64Var(&cfg.Seed, "seed", 1234567890, "Seed value for this Node")
	flag.StringVar(&cfg.ExperimentLogPath, "experiment-log", "", "Path to append experiment metric lines (JSON)")
	flag.StringVar(&cfg.NeighborsPolicy, "neighbors-policy", "first", "Fanout selection: 'first' or 'random'")
	flag.IntVar(&cfg.PullInterval, "pull-interval", 0, "IHAVE send interval in ms (0=disabled)")
	flag.IntVar(&cfg.IHaveMaxIds, "ihave-max-ids", 32, "Max message IDs per IHAVE message")
	flag.IntVar(&cfg.GossipCacheMaxSize, "gossip-cache-max", 0, "Max cached gossip payloads for IWANT (0=default 2000)")
	flag.IntVar(&cfg.PowK, "pow-k", 4, "PoW difficulty: leading hex-zero nibbles required in HELLO (0=disabled)")
	flag.Parse()

	return cfg, debug
}

func logLevel(debug bool) slog.Level {
	if debug {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func setupLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

func runCLI(ctx context.Context, n *node.Node, logger *slog.Logger) error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		n.PublishGossip("chat", line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	return nil
}

package main

import(
	"flag"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/node"
	"log/slog"
	"os"
)

func setupLogger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, 
	})

	logger := slog.New(handler)

	slog.SetDefault(logger)

	return logger
}

func main() {
	cfg := node.Config{}

	logger := setupLogger()

	flag.IntVar(&cfg.Port, "port", 8000, "Listening port for this Node")
	flag.StringVar(&cfg.Bootstrap, "bootstrap","", "Address of the seed Node")
	flag.IntVar(&cfg.Fanout, "fanout", 3, "Fanout value for this Node")
	flag.IntVar(&cfg.TTL, "ttl", 10, "TTL value for this Node")
	flag.IntVar(&cfg.PeerLimit, "peer-limit", 100, "Peer limit for this Node")
	flag.IntVar(&cfg.PingInterval, "ping-interval", 1000, "Ping interval in milliseconds for this Node")
	flag.IntVar(&cfg.PeerTimeout, "peer-timeout", 5000, "Peer timeout in milliseconds for this Node")
	flag.Int64Var(&cfg.Seed, "seed", 1234567890, "Seed value for this Node")

	flag.Parse()

	logger.Info("configuration loaded", slog.Any("config", cfg))

	n, err := node.New(cfg)
	if err != nil {
		logger.Error("failed to initialize node", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if cfg.Bootstrap != "" {
		logger.Info("starting as peer", slog.String("bootstrap", cfg.Bootstrap), slog.Int("port", cfg.Port))
	} else {
		logger.Info("starting as seed node", slog.Int("port", cfg.Port))
	}

	if err := n.Start(); err != nil {
		logger.Error("node exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
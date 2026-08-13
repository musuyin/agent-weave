package sandbox

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/docker/docker/client"

	"github/musuyin/agent-weave/internal/config"
)

// ProvideManager constructs the sandbox Manager and attempts to pre-pull the configured
// image. Returns a cleanup func that stops all containers on server shutdown.
// Wire provider.
func ProvideManager(ctx context.Context, cfg *config.Config, log *slog.Logger) (*Manager, func(), error) {
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, nil, err
	}

	baseDir, err := filepath.Abs(cfg.Sandbox.WorkspaceDir)
	if err != nil {
		dockerClient.Close()
		return nil, nil, err
	}

	mgr := &Manager{
		docker:     dockerClient,
		image:      cfg.Sandbox.Image,
		baseDir:    baseDir,
		containers: make(map[string]*Container),
		log:        log,
	}

	// Pull image in background so startup is not blocked.
	go mgr.pullImage(context.Background())

	cleanup := func() {
		if err := mgr.Close(); err != nil {
			log.Warn("sandbox close error", "error", err)
		}
	}
	return mgr, cleanup, nil
}

package workspace

import (
	"context"
	"log/slog"
	"os"

	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/events"
)

type WorkloadInitializer struct {
	repo        *Repository
	portManager *PortManager
}

func NewWorkloadInitializer(repo *Repository, portManager *PortManager) *WorkloadInitializer {
	return &WorkloadInitializer{repo, portManager}
}

func (wi *WorkloadInitializer) Handle(ctx context.Context, envelope events.Envelope) error {
	if envelope.IsReplay() {
		return nil
	}
	switch envelope.Event().(type) {
	case *WorkspaceCreated:
		if workspace, err := wi.repo.LoadById(ctx, envelope.AggregateId()); err == nil {
			if err := os.MkdirAll(workspace.workspaceDir(), os.ModePerm); err != nil {
				slog.Default().Error("failed to create workspace directory", "error", err)
				return err
			}
			host := config.GetEnv("OPENCODE_REMOTE_HOST", "127.0.0.1")
			if err = workspace.CreateSshWorkload(host, wi.portManager); err != nil {
				return err
			} else if err := workspace.CreateOpencodeWorkload(host, wi.portManager); err != nil {
				return err
			} else if err = wi.repo.Save(ctx, workspace); err != nil {
				return err
			}
		}
	}
	return nil
}

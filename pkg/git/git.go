package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"gtihub.com/mwildt/git-works/pkg/gitlab"
)

type Git struct {
	maxRetries   int
	retryDelay   time.Duration
	cloneTimeout time.Duration
}

func NewGit() *Git {
	return &Git{
		maxRetries:   3,
		retryDelay:   5 * time.Second,
		cloneTimeout: 30 * time.Second,
	}
}

func (g *Git) Clone(token gitlab.Token, url string, path string, branch string) error {
	slog.Info("cloning", "url", url, "to", path, "branch", branch)

	var lastErr error
	for i := 0; i <= g.maxRetries; i++ {
		if i > 0 {
			slog.Info("retrying clone", "attempt", i+1, "url", url)
			time.Sleep(g.retryDelay)
		}

		ctx, cancel := context.WithTimeout(context.Background(), g.cloneTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--depth", "1", url, path)
		cmd.Env = append(os.Environ(),
			"SSH_ASKPASS=/home/malte/projects/git-works/git-askpass",
			"SSH_ASKPASS_REQUIRE=force",
			fmt.Sprintf("GIT_TOKEN=%s", token.Value),
		)
		output, err := cmd.CombinedOutput()

		if err == nil {
			slog.Info("project cloned success", "url", url, "to", path, "branch", branch)
			return nil
		}

		lastErr = err
		slog.Warn("clone attempt failed", "attempt", i+1, "error", err, "output", string(output))

		if ctx.Err() == context.DeadlineExceeded {
			break
		}
	}

	outputStr := strings.TrimSpace(string([]byte{}))
	if lastErr != nil {
		if outputStr != "" {
			return fmt.Errorf("git clone failed after %d attempts: %s (output: %s)", g.maxRetries+1, lastErr.Error(), outputStr)
		}
		return fmt.Errorf("git clone failed after %d attempts: %s", g.maxRetries+1, lastErr.Error())
	}
	return nil
}

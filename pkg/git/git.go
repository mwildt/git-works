package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
)

type Git struct {
	maxRetries   int
	retryDelay   time.Duration
	cloneTimeout time.Duration
}

type Commit struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Author  string `json:"author"`
	Message string `json:"message"`
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
			"SSH_ASKPASS_REQUIRE=force",
			fmt.Sprintf("SSH_ASKPASS=%s", config.GetEnv("SSH_ASKPASS", "/usr/local/bin/git-askpass")),
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

func (g *Git) CommitAndPush(token gitlab.Token, dir string, message string, branch string) error {
	if err := g.Commit(dir, message); err != nil {
		return err
	}
	return g.Push(token, dir, branch)
}

func (g *Git) Commit(dir string, message string) error {
	if !isGitRepo(dir) {
		return fmt.Errorf("unable to commit, %s is not a git repo", dir)
	}

	steps := [][]string{
		{"add", "-A"},
		{"commit", "-m", message},
	}
	for _, args := range steps {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = os.Environ()
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s failed: %s (%s)", args[0], err.Error(), strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func (g *Git) Push(token gitlab.Token, dir string, branch string) error {
	if !isGitRepo(dir) {
		return fmt.Errorf("unable to push, %s is not a git repo", dir)
	}

	if branch == "" {
		branch = "HEAD"
	} else if current, err := currentBranch(dir); err == nil && branch != current {
		cmd := exec.Command("git", "-C", dir, "checkout", "-B", branch)
		cmd.Env = askpassEnv(token)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout failed: %s (%s)", err.Error(), strings.TrimSpace(string(output)))
		}
	}

	slog.Info("pushing", "dir", dir, "branch", branch)
	cmd := exec.Command("git", "-C", dir, "push", "-u", "origin", branch)
	cmd.Env = askpassEnv(token)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %s (%s)", err.Error(), strings.TrimSpace(string(output)))
	}
	return nil
}

func askpassEnv(token gitlab.Token) []string {
	return append(os.Environ(),
		"SSH_ASKPASS_REQUIRE=force",
		fmt.Sprintf("SSH_ASKPASS=%s", config.GetEnv("SSH_ASKPASS", "/usr/local/bin/git-askpass")),
		fmt.Sprintf("GIT_TOKEN=%s", token.Value),
	)
}

func (g *Git) CommitsAhead(dir string, branch string) ([]Commit, error) {
	if !isGitRepo(dir) {
		return []Commit{}, nil
	}
	if branch == "" {
		branch = "HEAD"
	}
	rangeRef := fmt.Sprintf("%s..HEAD", branch)
	cmd := exec.Command("git", "-C", dir, "log", "--pretty=format:%H%x1f%h%x1f%an%x1f%s", rangeRef)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s failed: %s", rangeRef, err.Error())
	}
	commits := []Commit{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    parts[0],
			Short:   parts[1],
			Author:  parts[2],
			Message: parts[3],
		})
	}
	return commits, nil
}

func currentBranch(dir string) (string, error) {
	output, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(path.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

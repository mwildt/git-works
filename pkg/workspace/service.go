package workspace

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/events"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
)

type ProjectStatus struct {
	ProjectId     uuid.UUID    `json:"project_id"`
	Initialized   bool         `json:"initialized"`
	HasChanges    bool         `json:"has_changes"`
	CurrentBranch string       `json:"current_branch"`
	Commits       []git.Commit `json:"commits"`
}

type Service struct {
	repo          *Repository
	gitClient     *git.Git
	actions       *actions.Client
	portManager   *PortManager
	tokenProvider *gitlab.TokenProvider
	dispatcher    *events.Dispatcher
}

func NewService(repo *Repository, gitClient *git.Git, actions *actions.Client, portManager *PortManager, tokenProvider *gitlab.TokenProvider, dispatcher *events.Dispatcher) *Service {
	return &Service{repo: repo, gitClient: gitClient, actions: actions, portManager: portManager, tokenProvider: tokenProvider, dispatcher: dispatcher}
}

func (s *Service) Create(ctx context.Context) (*Workspace, error) {
	owner, ok := auth.Owner(ctx)
	if !ok {
		return nil, Forbidden
	}

	workspace := NewWorkspace(owner)
	if err := os.MkdirAll(workspace.workspaceDir(), os.ModePerm); err != nil {
		slog.Default().Error("failed to create workspace directory", "error", err)
	}

	if err := s.repo.Save(ctx, workspace); err != nil {
		return nil, err
	}

	return workspace, nil
}

func (s *Service) ProjectStatus(_ context.Context, workspace *Workspace, project Project) (ProjectStatus, error) {
	projectDir := path.Join(workspace.workspaceDir(), project.Path)
	// den branch zu bestimmenen ist eigentlich deutlich komplizietrer....
	commits, err := s.gitClient.CommitsAhead(projectDir, "origin/"+project.Branch)
	if err != nil {
		slog.Default().Warn("unable to determine commits ahead", "error", err)
		commits = []git.Commit{}
	}
	return ProjectStatus{
		ProjectId:     project.Id,
		Initialized:   isGitRepo(projectDir),
		CurrentBranch: currentBranch(projectDir),
		HasChanges:    hasGitChanges(projectDir),
		Commits:       commits,
	}, nil

}

func (s *Service) WorkloadStatus(_ context.Context, workload Workload) (string, error) {
	return workload.Status(s.actions)
}

func (s *Service) WorkspaceInit(ctx context.Context, workspace Workspace) error {
	token, err := s.tokenProvider.TokenFor(ctx)
	if err != nil {
		return Forbidden
	}
	workspacePath := workspace.workspaceDir()
	for _, project := range workspace.Projects {
		if err := project.Init(token, s.gitClient, workspacePath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ProjectInit(ctx context.Context, workspace *Workspace, project Project) error {
	token, err := s.tokenProvider.TokenFor(ctx)
	if err != nil {
		return Forbidden
	}
	return project.Init(token, s.gitClient, workspace.workspaceDir())
}

func (s *Service) WorkloadInit(ctx context.Context, workspace *Workspace, workload Workload) error {
	return workload.Init(workspace.baseDir())
}

func (s *Service) WorkloadStart(ctx context.Context, workspace *Workspace, workload Workload) error {
	token, err := s.tokenProvider.TokenFor(ctx)
	if err != nil {
		return Forbidden
	}
	if err := workload.Start(token, s.actions, workspace.Id); err != nil {
		return err
	}
	s.dispatchRuntime(ctx, workspace.Id, &WorkloadActivity{WorkspaceId: workspace.Id, WorkloadId: workload.Id, At: time.Now()})
	return nil
}

func (s *Service) WorkloadStop(ctx context.Context, workspace *Workspace, workload Workload) error {
	token, err := s.tokenProvider.TokenFor(ctx)
	if err != nil {
		return Forbidden
	}
	if err := workload.Stop(token, s.actions, workspace.Id); err != nil {
		return err
	}
	s.dispatchRuntime(ctx, workspace.Id, &WorkloadActivity{WorkspaceId: workspace.Id, WorkloadId: workload.Id, At: time.Now()})
	return nil
}

// dispatchRuntime feuert ein nicht-persistentes Runtime-Event auf dem
// Dispatcher. Es wird nie an das Repository angehängt und landet damit
// nicht im Event-Log.
func (s *Service) dispatchRuntime(ctx context.Context, workspaceId uuid.UUID, event events.DomainEvent) {
	if s.dispatcher == nil {
		return
	}
	if err := s.dispatcher.Dispatch(ctx, NewRuntimeEnvelope(workspaceId, event, time.Now())); err != nil {
		slog.Default().Warn("unable to dispatch runtime event", "type", event.EventType(), "error", err)
	}
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	workspace, err := s.repo.byId(ctx, id)
	if err != nil {
		return err
	}

	// Save the workspace with workload delete events
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	go func() {
		workspacePath := workspace.baseDir()
		err := os.RemoveAll(workspacePath)
		if err != nil {
			slog.Default().Error("unable to remove workspace directory", "error", err)
		}
	}()
	return nil
}

func (s *Service) DeleteProject(ctx context.Context, workspaceId uuid.UUID, projectId uuid.UUID) error {
	workspace, project, err := s.repo.ProjectById(ctx, workspaceId, projectId)
	if err != nil {
		return err
	}

	if ok := workspace.deleteProjectById(projectId); !ok {
		return err
	}
	if err := s.repo.Save(ctx, workspace); err != nil {
		return err
	}
	go func() {
		workspacePath := workspace.workspaceDir()
		dataPath := path.Join(workspacePath, project.Path)
		err := os.RemoveAll(dataPath)
		if err != nil {
			slog.Default().Error("unable to remove workspace directory", "error", err)
		}
	}()
	return nil
}

func (s *Service) ProjectCommit(ctx context.Context, workspaceId uuid.UUID, projectId uuid.UUID, request CommitRequest) error {
	workspace, project, err := s.repo.ProjectById(ctx, workspaceId, projectId)
	if err != nil {
		return err
	}
	slog.Default().Info("commit", "workspaceId", workspaceId, "projectId", projectId)
	return s.gitClient.Commit(path.Join(workspace.workspaceDir(), project.Path), request.Message)
}

func (s *Service) ProjectPush(ctx context.Context, workspaceId uuid.UUID, projectId uuid.UUID, request PushRequest) error {
	workspace, project, err := s.repo.ProjectById(ctx, workspaceId, projectId)
	if err != nil {
		return err
	}
	token, err := s.tokenProvider.TokenFor(ctx)
	if err != nil {
		return Forbidden
	}
	slog.Default().Info("push", "workspaceId", workspaceId, "projectId", projectId, "branch", request.Branch)
	return s.gitClient.Push(token, path.Join(workspace.workspaceDir(), project.Path), request.Branch)
}

func (s *Service) ProjectCommitPush(ctx context.Context, workspaceId uuid.UUID, projectId uuid.UUID, request CommitRequest) error {
	workspace, project, err := s.repo.ProjectById(ctx, workspaceId, projectId)
	if err != nil {
		return err
	}
	token, err := s.tokenProvider.TokenFor(ctx)
	if err != nil {
		return Forbidden
	}
	slog.Default().Info("commit push", "workspaceId", workspaceId, "projectId", projectId)
	return s.gitClient.CommitAndPush(token, path.Join(workspace.workspaceDir(), project.Path), request.Message, "HEAD")
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(path.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func hasGitChanges(dir string) bool {
	if !isGitRepo(dir) {
		return false
	}
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("failed to check git status", "dir", dir, "error", err)
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

func currentBranch(dir string) string {
	if !isGitRepo(dir) {
		return ""
	}

	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("failed to get current branch", "dir", dir, "error", err)
		return ""
	}

	return strings.TrimSpace(string(output))

}

func (s *Service) CreateVscodeWorkload(ctx context.Context, workspace *Workspace) error {
	host := "127.0.0.1"
	if err := workspace.CreateVscodeWorkload(host, s.portManager); err != nil {
		return err
	}
	return s.repo.Save(ctx, workspace)
}

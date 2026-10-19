package workspace

//
//type (
//	Manager struct {
//		items map[string]*Workspace
//		log   *eventlog
//	}
//)

//func CreateOpencodeWorkload(ws Workspace, portProvider func() (int, error)) (workoad Workload, err error) {
//	port, err := portProvider()
//	if err != nil {
//		return
//	}
//
//	return Workload{
//		Id:   uuid.New(),
//		Type: "OpenCode",
//		Data: OpencodeConfig{
//			Key:          "workspaces_opencode_" + ws.Id.String()[0:10],
//			AuthUsername: "git-works",
//			AuthPassword: "password",
//			Host:         config.GetEnv("OPENCODE_REMOTE_HOST", "127.0.0.1"),
//			Port:         port,
//		},
//	}, nil
//}

//
//func (m *Manager) Create(owner string) (*Workspace, error) {
//	ws := NewWorkspace(owner)
//	if err := os.MkdirAll(ws.workspaceDir(), os.ModePerm); err != nil {
//		slog.Default().Error("failed to create workspace directory", "error", err)
//	}
//
//	workload, err := CreateOpencodeWorkload(ws, func() (int, error) {
//		return m.nextPort(), nil
//	})
//	if err != nil {
//		slog.Default().Error("unable to create opencode Workload ", "error", err)
//		if err := os.RemoveAll(ws.workspaceDir()); err != nil {
//			slog.Default().Warn("unable to rollback workspace", "error", err)
//		}
//		return nil, err
//	}
//	ws.Workloads = append(ws.Workloads, workload)
//	m.items[ws.Id.String()] = &ws
//	m.appendEvent("workspace.full", ws)
//
//	//go func() {
//	//	opencodeConfig := []byte("{}")
//	//	opencodeFile := path.Join(ws.baseDir(), "opencode.json")
//	//	if err := os.WriteFile(opencodeFile, opencodeConfig, 0644); err != nil {
//	//		slog.Default().Error("Error creating opencode file", "error", err)
//	//	}
//	//}()
//	return &ws, nil
//}
//
//func (m *Manager) ByIdAndOwner(id uuid.UUID, owner string) (res *Workspace, found bool) {
//	ws, ok := m.items[id.String()]
//	if !ok {
//		return nil, false
//
//	}
//	if ws.Owner != owner {
//		return nil, false
//	}
//	return ws, true
//}
//
//func (m *Manager) AllByOwner(owner string) (res []*Workspace) {
//	for _, w := range m.items {
//		if w.Owner == owner {
//			res = append(res, w)
//		}
//	}
//	return res
//}
//
//func (m *Manager) DeleteByIdAndOwner(id uuid.UUID, owner string) bool {
//	ws, ok := m.items[id.String()]
//	if !ok {
//		return false
//	}
//	if ws.Owner != owner {
//		return false
//	}
//	delete(m.items, ws.Id.String())
//	go m.appendEvent("workspace.delete", ws.Id)
//	return true
//}
//
//func (m *Manager) GetProjectStatus(username string, workspaceId uuid.UUID, projectId uuid.UUID) (res ProjectStatus, _ error) {
//	w, ok := m.ByIdAndOwner(workspaceId, username)
//	if !ok {
//		return res, errors.New("not found")
//	}
//	project, ok := w.ProjectById(projectId)
//	if !ok {
//		return res, errors.New("not found")
//	}
//
//	return ProjectStatus{
//		ProjectId:   project.Id,
//		Initialized: isGitRepo(path.Join(w.workspaceDir(), project.Path)),
//		HasChanges:  hasGitChanges(path.Join(w.workspaceDir(), project.Path)),
//	}, nil
//
//}
//
//func (m *Manager) nextPort() int {
//	port := config.GetEnvInt("WORKSPACE_START_PORT", 9000)
//	for _, ws := range m.items {
//		if ws.Opencode.Port >= port {
//			port = ws.Opencode.Port + 1
//		}
//	}
//	return port
//}
//
//func (m *Manager) AddProject(workspace *Workspace, p string, url string, name string) {
//	workspace.addProject(p, url, name)
//	go m.appendEvent("workspace.full", workspace)
//}
//
//func (m *Manager) DeleteProjectById(workspace *Workspace, id uuid.UUID) {
//	workspace.deleteProjectById(id)
//	go m.appendEvent("workspace.full", workspace)
//}
//
//func (m *Manager) DeleteWorkloadById(workspace *Workspace, id uuid.UUID) {
//	workspace.deleteWorkloadById(id)
//	go m.appendEvent("workspace.full", workspace)
//}

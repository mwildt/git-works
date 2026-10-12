package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/gitlab"
)

func RandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const (
	CookieName = "gw_session"
	sessionTTL = 12 * time.Hour
)

type Session struct {
	Id              string
	Token           gitlab.Token
	Username        string
	ActiveWorkspace uuid.UUID
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	client   *gitlab.Client
}

func (m *Manager) CreateSession(token gitlab.Token) (Session, error) {
	user, err := m.client.GetUser(token)
	if err != nil {
		slog.Default().Error("unable to get user", "error", err)
		return Session{}, err
	}
	id, err := RandomString(32)
	if err != nil {
		return Session{}, err
	}
	session := Session{
		Id:              id,
		Token:           token,
		Username:        user.Username,
		ActiveWorkspace: uuid.Nil(),
	}
	m.sessions[id] = &session
	return session, nil
}

func (m *Manager) Validate(value string) (Session, error) {
	sess, ok := m.sessions[value]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}
	return *sess, nil
}

func (m *Manager) ActivateWorkspace(sessionId string, wsid uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[sessionId]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.ActiveWorkspace = wsid
	return nil
}

func NewSessionManager(client *gitlab.Client) *Manager {
	return &Manager{
		client:   client,
		sessions: make(map[string]*Session),
		mu:       sync.RWMutex{},
	}
}

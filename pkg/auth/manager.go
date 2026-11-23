package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
	"uuid"
)

func RandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const (
	sessionTTL = 12 * time.Hour
)

type WorkloadReference struct {
	Location    string
	WorkspaceId uuid.UUID
	WorkloadId  uuid.UUID
}

func NilWorkloadReference() WorkloadReference {
	return WorkloadReference{}
}

type Session struct {
	Id             string
	Username       string
	ActiveWorkload WorkloadReference
	values         map[string]any
}

func (session Session) Value(key string) any {
	return session.values[key]
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func (sessionManager *Manager) CreateSession(username string) (Session, error) {
	id, err := RandomString(32)
	if err != nil {
		return Session{}, err
	}
	session := Session{
		Id:             id,
		Username:       username,
		ActiveWorkload: WorkloadReference{},
		values:         make(map[string]any),
	}
	sessionManager.sessions[id] = &session
	return session, nil
}

func (sessionManager *Manager) Validate(value string) (Session, error) {
	sess, ok := sessionManager.sessions[value]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}
	return *sess, nil
}

func (sessionManager *Manager) ActivateWorkspace(sessionId string, workload WorkloadReference) error {
	sessionManager.mu.Lock()
	defer sessionManager.mu.Unlock()
	sess, ok := sessionManager.sessions[sessionId]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.ActiveWorkload = workload
	return nil
}

func (sessionManager *Manager) Authenticated(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		cookie, err := r.Cookie("gw_session")
		if err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(w, "invalid cookie", http.StatusUnauthorized)
			return
		}
		session, err := sessionManager.Validate(cookie.Value)
		if err != nil {
			slog.Default().Warn("session is invalid")
			http.Error(w, "invalid cookie", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(WithSession(r.Context(), session))
		handler.ServeHTTP(w, r)
	}
}

func (sessionManager *Manager) PutValue(id string, key string, value any) bool {
	session, ok := sessionManager.sessions[id]
	if !ok {
		return false
	}
	session.values[key] = value
	return true
}

func (sessionManager *Manager) Delete(id string) {
	slog.Default().Warn("session is deleted", "id", id)
	delete(sessionManager.sessions, id)
}

func NewSessionManager() *Manager {
	return &Manager{
		//client:   client,
		sessions: make(map[string]*Session),
		mu:       sync.RWMutex{},
	}
}

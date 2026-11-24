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

const (
	AuthMethodToken = "token"
	AuthMethodOIDC  = "oidc"
)

type WorkloadReference struct {
	Location    string
	WorkspaceId uuid.UUID
	WorkloadId  uuid.UUID
}

func NilWorkloadReference() WorkloadReference {
	return WorkloadReference{}
}

type OIDCTokens struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

type Session struct {
	Id             string
	Username       string
	AuthMethod     string
	ExpiresAt      time.Time
	ActiveWorkload WorkloadReference
	OIDC           *OIDCTokens
	values         map[string]any
}

func (session Session) Value(key string) any {
	return session.values[key]
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func (sessionManager *Manager) CreateSession(username string, authMethod string) (Session, error) {
	id, err := RandomString(32)
	if err != nil {
		return Session{}, err
	}
	session := Session{
		Id:             id,
		Username:       username,
		AuthMethod:     authMethod,
		ExpiresAt:      time.Now().Add(sessionTTL),
		ActiveWorkload: WorkloadReference{},
		values:         make(map[string]any),
	}
	sessionManager.mu.Lock()
	sessionManager.sessions[id] = &session
	sessionManager.mu.Unlock()
	return session, nil
}

func (sessionManager *Manager) Validate(value string) (Session, error) {
	sessionManager.mu.RLock()
	sess, ok := sessionManager.sessions[value]
	if !ok {
		sessionManager.mu.RUnlock()
		return Session{}, fmt.Errorf("session not found")
	}
	expired := !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt)
	session := *sess
	sessionManager.mu.RUnlock()
	if expired {
		sessionManager.Delete(value)
		return Session{}, fmt.Errorf("session expired")
	}
	return session, nil
}

func (sessionManager *Manager) Touch(id string) {
	sessionManager.mu.Lock()
	defer sessionManager.mu.Unlock()
	if sess, ok := sessionManager.sessions[id]; ok && !sess.ExpiresAt.IsZero() {
		sess.ExpiresAt = time.Now().Add(sessionTTL)
	}
}

func (sessionManager *Manager) SetOIDC(id string, tokens OIDCTokens) bool {
	sessionManager.mu.Lock()
	defer sessionManager.mu.Unlock()
	sess, ok := sessionManager.sessions[id]
	if !ok {
		return false
	}
	sess.OIDC = &tokens
	return true
}

func (sessionManager *Manager) OIDCTokens(id string) (OIDCTokens, bool) {
	sessionManager.mu.RLock()
	defer sessionManager.mu.RUnlock()
	sess, ok := sessionManager.sessions[id]
	if !ok || sess.OIDC == nil {
		return OIDCTokens{}, false
	}
	return *sess.OIDC, true
}

func (sessionManager *Manager) RefreshOIDC(id string, refresh func(current OIDCTokens) (OIDCTokens, error)) (OIDCTokens, error) {
	sessionManager.mu.Lock()
	defer sessionManager.mu.Unlock()
	sess, ok := sessionManager.sessions[id]
	if !ok {
		return OIDCTokens{}, fmt.Errorf("session not found")
	}
	if sess.OIDC == nil {
		return OIDCTokens{}, fmt.Errorf("no oidc tokens in session")
	}
	tokens, err := refresh(*sess.OIDC)
	if err != nil {
		return OIDCTokens{}, err
	}
	sess.OIDC = &tokens
	if !sess.ExpiresAt.IsZero() {
		sess.ExpiresAt = time.Now().Add(sessionTTL)
	}
	return tokens, nil
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
		sessionManager.Touch(session.Id)
		r = r.WithContext(WithSession(r.Context(), session))
		handler.ServeHTTP(w, r)
	}
}

func (sessionManager *Manager) PutValue(id string, key string, value any) bool {
	sessionManager.mu.Lock()
	defer sessionManager.mu.Unlock()
	session, ok := sessionManager.sessions[id]
	if !ok {
		return false
	}
	session.values[key] = value
	return true
}

func (sessionManager *Manager) Delete(id string) {
	slog.Default().Warn("session is deleted", "id", id)
	sessionManager.mu.Lock()
	defer sessionManager.mu.Unlock()
	delete(sessionManager.sessions, id)
}

func NewSessionManager() *Manager {
	return &Manager{
		//client:   client,
		sessions: make(map[string]*Session),
		mu:       sync.RWMutex{},
	}
}

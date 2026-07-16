package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const (
	dashboardCookieName = "cortex_dashboard"
	dashboardSessionTTL = 8 * time.Hour
)

type dashboardSession struct {
	AgentID   string
	CSRFToken string
	ExpiresAt time.Time
}

type dashboardSessions struct {
	mu       sync.Mutex
	sessions map[string]dashboardSession
}

func newDashboardSessions() *dashboardSessions {
	return &dashboardSessions{sessions: make(map[string]dashboardSession)}
}

func (sessions *dashboardSessions) create(agentID string) (string, dashboardSession, error) {
	sessionID, err := secureToken()
	if err != nil {
		return "", dashboardSession{}, err
	}
	csrfToken, err := secureToken()
	if err != nil {
		return "", dashboardSession{}, err
	}
	session := dashboardSession{AgentID: agentID, CSRFToken: csrfToken, ExpiresAt: time.Now().Add(dashboardSessionTTL)}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	sessions.deleteExpiredLocked(time.Now())
	sessions.sessions[sessionID] = session
	return sessionID, session, nil
}

func (sessions *dashboardSessions) fromRequest(request *http.Request) (string, dashboardSession, bool) {
	cookie, err := request.Cookie(dashboardCookieName)
	if err != nil || cookie.Value == "" {
		return "", dashboardSession{}, false
	}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	session, ok := sessions.sessions[cookie.Value]
	if !ok || !session.ExpiresAt.After(time.Now()) {
		delete(sessions.sessions, cookie.Value)
		return "", dashboardSession{}, false
	}
	return cookie.Value, session, true
}

func (sessions *dashboardSessions) delete(sessionID string) {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	delete(sessions.sessions, sessionID)
}

func (sessions *dashboardSessions) deleteExpiredLocked(now time.Time) {
	for sessionID, session := range sessions.sessions {
		if !session.ExpiresAt.After(now) {
			delete(sessions.sessions, sessionID)
		}
	}
}

func secureToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

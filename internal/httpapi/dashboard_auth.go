package httpapi

import (
	"net"
	"net/http"
	"strings"
	"time"
)

type dashboardAccess interface {
	DashboardAccess() (agentID string, passwordless bool)
}

func (server *Server) dashboardSession(
	writer http.ResponseWriter,
	request *http.Request,
) (dashboardSession, bool) {
	_, session, ok := server.sessions.fromRequest(request)
	if ok {
		return session, true
	}
	access, supported := server.auth.(dashboardAccess)
	if !supported || !isLoopbackRequest(request) {
		return dashboardSession{}, false
	}
	agentID, passwordless := access.DashboardAccess()
	if !passwordless || agentID == "" {
		return dashboardSession{}, false
	}
	if err := server.establishDashboardSessionAt(writer, request, agentID, request.URL.RequestURI()); err != nil {
		http.Error(writer, "create local session", http.StatusInternalServerError)
	}
	return dashboardSession{}, false
}

func isLoopbackRequest(request *http.Request) bool {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (server *Server) login(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4096)
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid login", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(request.FormValue("token"))
	agentID, ok := authenticateDashboard(server.auth, token)
	if !ok {
		http.Error(writer, "invalid token", http.StatusUnauthorized)
		return
	}
	if err := server.establishDashboardSession(writer, request, agentID); err != nil {
		http.Error(writer, "create session", http.StatusInternalServerError)
	}
}

func (server *Server) establishDashboardSession(
	writer http.ResponseWriter,
	request *http.Request,
	agentID string,
) error {
	return server.establishDashboardSessionAt(writer, request, agentID, "/")
}

func (server *Server) establishDashboardSessionAt(
	writer http.ResponseWriter,
	request *http.Request,
	agentID string,
	target string,
) error {
	sessionID, session, err := server.sessions.create(agentID)
	if err != nil {
		return err
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     dashboardCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
		MaxAge:   int(dashboardSessionTTL / time.Second),
	})
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		target = "/"
	}
	http.Redirect(writer, request, target, http.StatusSeeOther)
	return nil
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request) {
	sessionID, session, ok := server.sessions.fromRequest(request)
	if !ok {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !validCSRF(session.CSRFToken, request.FormValue("csrf")) {
		http.Error(writer, "invalid csrf token", http.StatusForbidden)
		return
	}
	server.sessions.delete(sessionID)
	http.SetCookie(writer, &http.Cookie{
		Name: dashboardCookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

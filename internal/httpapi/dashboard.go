package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"cortex.local/cortex/internal/cortex"
)

//go:embed templates/*.html static/*.css
var dashboardAssets embed.FS

var dashboardTemplates = template.Must(template.ParseFS(dashboardAssets, "templates/*.html"))

type dashboardView struct {
	AgentID   string
	CSRFToken string
	Total     int
	Candidate int
	Active    int
	Canonical int
	Memories  []dashboardMemory
}

type dashboardMemory struct {
	cortex.Memory
	CanApprove   bool
	CanPromote   bool
	CanReject    bool
	CanSupersede bool
	CanArchive   bool
}

type dashboardDetailView struct {
	AgentID   string
	CSRFToken string
	Memory    dashboardMemory
	Events    []dashboardEvent
}

type dashboardEvent struct {
	cortex.Event
	MetadataJSON string
}

func (server *Server) dashboard(writer http.ResponseWriter, request *http.Request) {
	setDashboardHeaders(writer)
	_, session, ok := server.sessions.fromRequest(request)
	if !ok {
		writer.Header().Set("Cache-Control", "no-store")
		if err := dashboardTemplates.ExecuteTemplate(writer, "login.html", nil); err != nil {
			http.Error(writer, "render login", http.StatusInternalServerError)
		}
		return
	}
	overview, err := server.hub.Overview(request.Context(), session.AgentID, 200)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	view := dashboardView{
		AgentID:   session.AgentID,
		CSRFToken: session.CSRFToken,
		Candidate: overview.Counts[cortex.LifecycleCandidate],
		Active:    overview.Counts[cortex.LifecycleActive],
		Canonical: overview.Counts[cortex.LifecycleCanonical],
		Memories:  make([]dashboardMemory, 0, len(overview.Memories)),
	}
	for _, count := range overview.Counts {
		view.Total += count
	}
	for _, memory := range overview.Memories {
		item := dashboardMemory{Memory: memory, CanArchive: memory.Lifecycle != cortex.LifecycleArchived}
		item.CanApprove = memory.Lifecycle == cortex.LifecycleCandidate
		item.CanPromote = memory.Lifecycle == cortex.LifecycleActive
		item.CanReject = memory.Lifecycle == cortex.LifecycleCandidate || memory.Lifecycle == cortex.LifecycleActive
		item.CanSupersede = memory.Lifecycle != cortex.LifecycleSuperseded && memory.Lifecycle != cortex.LifecycleArchived
		view.Memories = append(view.Memories, item)
	}
	writer.Header().Set("Cache-Control", "no-store")
	if err := dashboardTemplates.ExecuteTemplate(writer, "dashboard.html", view); err != nil {
		http.Error(writer, "render dashboard", http.StatusInternalServerError)
	}
}

func (server *Server) login(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4096)
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid login", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(request.FormValue("token"))
	agentID, ok := server.auth.Authenticate(token)
	if !ok {
		http.Error(writer, "invalid token", http.StatusUnauthorized)
		return
	}
	sessionID, session, err := server.sessions.create(agentID)
	if err != nil {
		http.Error(writer, "create session", http.StatusInternalServerError)
		return
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
	http.Redirect(writer, request, "/", http.StatusSeeOther)
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

func (server *Server) dashboardReview(writer http.ResponseWriter, request *http.Request) {
	_, session, ok := server.sessions.fromRequest(request)
	if !ok {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 8192)
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid review", http.StatusBadRequest)
		return
	}
	if !validCSRF(session.CSRFToken, request.FormValue("csrf")) {
		http.Error(writer, "invalid csrf token", http.StatusForbidden)
		return
	}
	requestID, err := dashboardRequestID()
	if err != nil {
		http.Error(writer, "create review id", http.StatusInternalServerError)
		return
	}
	_, err = server.hub.Review(request.Context(), cortex.ReviewCommand{
		IdempotencyKey: requestID,
		MemoryID:       request.PathValue("memoryID"),
		ActorID:        session.AgentID,
		Decision:       cortex.ReviewDecision(request.FormValue("decision")),
		Reason:         strings.TrimSpace(request.FormValue("reason")),
	})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	redirectTo := "/"
	if expected := "/ui/memories/" + request.PathValue("memoryID"); request.FormValue("return_to") == expected {
		redirectTo = expected
	}
	http.Redirect(writer, request, redirectTo, http.StatusSeeOther)
}

func (server *Server) dashboardDetail(writer http.ResponseWriter, request *http.Request) {
	setDashboardHeaders(writer)
	_, session, ok := server.sessions.fromRequest(request)
	if !ok {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	memory, events, err := server.hub.Inspect(request.Context(), cortex.HistoryQuery{
		MemoryID: request.PathValue("memoryID"), AgentID: session.AgentID,
	})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	item := dashboardMemory{Memory: memory, CanArchive: memory.Lifecycle != cortex.LifecycleArchived}
	item.CanApprove = memory.Lifecycle == cortex.LifecycleCandidate
	item.CanPromote = memory.Lifecycle == cortex.LifecycleActive
	item.CanReject = memory.Lifecycle == cortex.LifecycleCandidate || memory.Lifecycle == cortex.LifecycleActive
	item.CanSupersede = memory.Lifecycle != cortex.LifecycleSuperseded && memory.Lifecycle != cortex.LifecycleArchived
	view := dashboardDetailView{AgentID: session.AgentID, CSRFToken: session.CSRFToken, Memory: item}
	for _, event := range events {
		metadata, _ := json.Marshal(event.Metadata)
		view.Events = append(view.Events, dashboardEvent{Event: event, MetadataJSON: string(metadata)})
	}
	writer.Header().Set("Cache-Control", "no-store")
	if err := dashboardTemplates.ExecuteTemplate(writer, "detail.html", view); err != nil {
		http.Error(writer, "render memory detail", http.StatusInternalServerError)
	}
}

func dashboardRequestID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate dashboard request id: %w", err)
	}
	return "dashboard/review/" + hex.EncodeToString(raw[:]), nil
}

func validCSRF(expected, supplied string) bool {
	return expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

func setDashboardHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "no-referrer")
}

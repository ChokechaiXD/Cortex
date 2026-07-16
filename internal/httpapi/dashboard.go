package httpapi

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"cortex.local/cortex/internal/cortex"
)

const dashboardCookieName = "cortex_token"

//go:embed templates/*.html static/*.css
var dashboardAssets embed.FS

var dashboardTemplates = template.Must(template.ParseFS(dashboardAssets, "templates/*.html"))

type dashboardView struct {
	AgentID   string
	Total     int
	Candidate int
	Active    int
	Canonical int
	Memories  []dashboardMemory
}

type dashboardMemory struct {
	cortex.Memory
	CanApprove bool
	CanPromote bool
	CanReject  bool
	CanArchive bool
}

func (server *Server) dashboard(writer http.ResponseWriter, request *http.Request) {
	setDashboardHeaders(writer)
	agentID, ok := server.dashboardIdentity(request)
	if !ok {
		writer.Header().Set("Cache-Control", "no-store")
		if err := dashboardTemplates.ExecuteTemplate(writer, "login.html", nil); err != nil {
			http.Error(writer, "render login", http.StatusInternalServerError)
		}
		return
	}
	overview, err := server.hub.Overview(request.Context(), agentID, 200)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	view := dashboardView{
		AgentID:   agentID,
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
	if _, ok := server.auth.Authenticate(token); !ok {
		http.Error(writer, "invalid token", http.StatusUnauthorized)
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     dashboardCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name: dashboardCookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

func (server *Server) dashboardReview(writer http.ResponseWriter, request *http.Request) {
	agentID, ok := server.dashboardIdentity(request)
	if !ok {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 8192)
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid review", http.StatusBadRequest)
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
		ActorID:        agentID,
		Decision:       cortex.ReviewDecision(request.FormValue("decision")),
		Reason:         strings.TrimSpace(request.FormValue("reason")),
	})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

func (server *Server) dashboardIdentity(request *http.Request) (string, bool) {
	token := authenticationToken(request)
	if token == "" {
		return "", false
	}
	return server.auth.Authenticate(token)
}

func dashboardRequestID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate dashboard request id: %w", err)
	}
	return "dashboard/review/" + hex.EncodeToString(raw[:]), nil
}

func setDashboardHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "no-referrer")
}

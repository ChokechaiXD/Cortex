package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"cortex.local/cortex/internal/controlcenter"
	"cortex.local/cortex/internal/controlplane"
	"cortex.local/cortex/internal/cortex"
	hopemem "cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/intelligence"
	"cortex.local/cortex/internal/localauth"
)

type Server struct {
	hub      *cortex.Hub
	auth     Authenticator
	sessions *dashboardSessions
	control  runtimeControl
	launcher *localauth.Broker
	advisor  intelligence.Advisor
	hope     *controlplane.Plane
	skillMem skillMemory
}

func New(hub *cortex.Hub, auth Authenticator) http.Handler {
	return NewWithControl(hub, auth, nil)
}

type runtimeControl interface {
	Status(context.Context) (controlcenter.Status, error)
	Request(controlcenter.Action) error
	SyncHermes(context.Context) (controlcenter.SyncResult, error)
}

func NewWithControl(hub *cortex.Hub, auth Authenticator, control runtimeControl) http.Handler {
	return NewWithControlAndLauncher(hub, auth, control, nil)
}

func NewWithControlAndLauncher(
	hub *cortex.Hub,
	auth Authenticator,
	control runtimeControl,
	launcher *localauth.Broker,
) http.Handler {
	return NewWithControlLauncherAndAdvisor(hub, auth, control, launcher, nil)
}

func NewWithControlLauncherAndAdvisor(
	hub *cortex.Hub,
	auth Authenticator,
	control runtimeControl,
	launcher *localauth.Broker,
	advisor intelligence.Advisor,
) http.Handler {
	return newHandler(hub, auth, control, launcher, advisor, nil, nil)
}

func NewWithSkillMem(
	hub *cortex.Hub,
	auth Authenticator,
	control runtimeControl,
	launcher *localauth.Broker,
	advisor intelligence.Advisor,
	skillMem *hopemem.Hub,
) http.Handler {
	return newHandler(hub, auth, control, launcher, advisor, nil, skillMem)
}

func NewWithHOPE(
	hub *cortex.Hub,
	auth Authenticator,
	control runtimeControl,
	launcher *localauth.Broker,
	advisor intelligence.Advisor,
	hopePlane *controlplane.Plane,
) http.Handler {
	return newHandler(hub, auth, control, launcher, advisor, hopePlane, nil)
}

func newHandler(
	hub *cortex.Hub,
	auth Authenticator,
	control runtimeControl,
	launcher *localauth.Broker,
	advisor intelligence.Advisor,
	hopePlane *controlplane.Plane,
	skillMem *hopemem.Hub,
) http.Handler {
	server := &Server{
		hub: hub, auth: auth, sessions: newDashboardSessions(), control: control,
		launcher: launcher, advisor: advisor, hope: hopePlane, skillMem: skillMem,
	}
	mux := http.NewServeMux()
	staticFiles, _ := fs.Sub(dashboardAssets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))
	mux.HandleFunc("GET /", server.dashboard)
	mux.HandleFunc("GET /knowledge", server.dashboard)
	mux.HandleFunc("POST /login", server.login)
	mux.HandleFunc("POST /logout", server.logout)
	mux.HandleFunc("POST /v1/dashboard/sessions", server.issueDashboardSession)
	mux.HandleFunc("GET /ui/session", server.consumeDashboardSession)
	mux.HandleFunc("POST /ui/system/action", server.systemAction)
	mux.HandleFunc("POST /ui/hermes/sync", server.hermesSync)
	mux.HandleFunc("POST /ui/hermes/settings", server.hermesSettings)
	mux.HandleFunc("POST /ui/curator/settings", server.curatorSettings)
	mux.HandleFunc("POST /ui/curator/run", server.curatorRun)
	mux.HandleFunc("POST /ui/advisor/settings", server.advisorSettings)
	mux.HandleFunc("POST /ui/advisor/run", server.advisorRun)
	mux.HandleFunc("POST /ui/hope/work-modes/{modeID}", server.hopeWorkMode)
	mux.HandleFunc("POST /ui/hope/work-modes", server.hopeSaveWorkMode)
	mux.HandleFunc("POST /ui/hope/integrations/{integrationID}", server.hopeIntegrationAction)
	mux.HandleFunc("POST /ui/hope/agents", server.hopeSaveAgent)
	mux.HandleFunc("POST /ui/hope/agents/{agentID}", server.hopeAgentAction)
	mux.HandleFunc("POST /ui/hope/projects/roots", server.hopeAddProjectRoot)
	mux.HandleFunc("POST /ui/hope/projects", server.hopeSaveProject)
	mux.HandleFunc("POST /ui/hope/projects/discover", server.hopeDiscoverProjects)
	mux.HandleFunc("POST /ui/hope/projects/{projectID}/open", server.hopeOpenProject)
	mux.HandleFunc("POST /ui/hope/projects/{projectID}/delete", server.hopeDeleteProject)
	mux.HandleFunc("POST /ui/hope/skills/sync", server.hopeSyncSkills)
	mux.HandleFunc("POST /ui/hope/skills", server.hopeCreateSkill)
	mux.HandleFunc("POST /ui/hope/skills/import", server.hopeImportSkill)
	mux.HandleFunc("POST /ui/hope/skills/{skillID}", server.hopeUpdateSkill)
	mux.HandleFunc("POST /ui/hope/skills/{skillID}/deploy", server.hopeDeploySkill)
	mux.HandleFunc("POST /ui/hope/skills/route", server.hopeRouteSkills)
	mux.HandleFunc("POST /ui/hope/automations/{jobID}", server.hopeAutomationAction)
	mux.HandleFunc("POST /ui/hope/security", server.hopeSecurity)
	mux.HandleFunc("GET /ui/memories/{memoryID}", server.dashboardDetail)
	mux.HandleFunc("POST /ui/memories/{memoryID}/review", server.dashboardReview)
	mux.HandleFunc("POST /ui/memories/review-batch", server.dashboardReviewBatch)
	mux.HandleFunc("GET /v1/health", server.health)
	mux.Handle("GET /v1/capabilities", server.authenticated(http.HandlerFunc(server.capabilities)))
	mux.Handle("POST /v1/memories", server.authenticated(http.HandlerFunc(server.remember)))
	mux.Handle("POST /v1/recalls", server.authenticated(http.HandlerFunc(server.recall)))
	mux.Handle("POST /v1/context-packs", server.authenticated(http.HandlerFunc(server.contextPack)))
	mux.Handle("POST /v1/context-packs/{packID}/skills/{skillID}/feedback", server.authenticated(http.HandlerFunc(server.contextSkillFeedback)))
	mux.Handle("POST /v1/memories/{memoryID}/feedback", server.authenticated(http.HandlerFunc(server.feedback)))
	mux.Handle("POST /v1/memories/{memoryID}/review", server.authenticated(http.HandlerFunc(server.review)))
	mux.Handle("GET /v1/memories/{memoryID}/history", server.authenticated(http.HandlerFunc(server.history)))
	return mux
}

func (server *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := bearerToken(request)
		agentID, ok := server.auth.Authenticate(token)
		if !ok || strings.TrimSpace(agentID) == "" {
			writeAPIError(writer, http.StatusUnauthorized, "unauthorized", "valid bearer token required")
			return
		}
		next.ServeHTTP(writer, request.WithContext(withIdentity(request.Context(), agentID)))
	})
}

func (server *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (server *Server) capabilities(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"version":    "v1",
		"agent_id":   identityFromRequest(request),
		"operations": []string{"remember", "recall", "context_pack", "feedback", "review", "history"},
		"search":     []string{"fts5"},
		"scopes":     []cortex.Scope{cortex.ScopeGlobal, cortex.ScopeProject, cortex.ScopeDomain, cortex.ScopePrivate},
	})
}

func idempotencyKey(writer http.ResponseWriter, request *http.Request) (string, bool) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" {
		writeAPIError(writer, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return "", false
	}
	return key, true
}

func writeDomainError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cortex.ErrInvalidInput):
		writeAPIError(writer, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, cortex.ErrForbidden):
		writeAPIError(writer, http.StatusForbidden, "forbidden", "operation is not permitted")
	case errors.Is(err, cortex.ErrNotFound):
		writeAPIError(writer, http.StatusNotFound, "not_found", "memory not found")
	case errors.Is(err, cortex.ErrConflict):
		writeAPIError(writer, http.StatusConflict, "conflict", err.Error())
	default:
		writeAPIError(writer, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

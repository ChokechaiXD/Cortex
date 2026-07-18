package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"cortex.local/cortex/internal/controlplane"
	"cortex.local/cortex/internal/hope"
)

func (server *Server) hopeAddProjectRoot(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if err := server.hope.AddProjectRoot(request.Context(), request.FormValue("path")); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?section=projects&notice=root-added", http.StatusSeeOther)
}

func (server *Server) hopeDiscoverProjects(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if _, err := server.hope.DiscoverProjects(request.Context()); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(writer, request, "/?section=projects&notice=projects-found", http.StatusSeeOther)
}

func (server *Server) hopeOpenProject(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if err := server.hope.OpenProject(request.Context(), request.PathValue("projectID")); err != nil {
		http.Error(writer, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(writer, request, "/?section=projects", http.StatusSeeOther)
}

func (server *Server) hopeSaveProject(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	progress, _ := strconv.Atoi(request.FormValue("progress"))
	project := hope.Project{
		ID: request.FormValue("id"), Name: request.FormValue("name"), Path: request.FormValue("path"),
		Kind: request.FormValue("kind"), Description: request.FormValue("description"),
		Goal: request.FormValue("goal"), Status: request.FormValue("status"),
		Progress: progress, CurrentState: request.FormValue("current_state"),
		NextAction: request.FormValue("next_action"), AgentIDs: controlplane.ParseKeywords(strings.Join(request.Form["agents"], ",")),
		Active: request.FormValue("active") == "yes",
	}
	if err := server.hope.SaveProject(request.Context(), project); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?section=projects&notice=project-saved", http.StatusSeeOther)
}

func (server *Server) hopeDeleteProject(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if err := server.hope.DeleteProject(request.Context(), request.PathValue("projectID")); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?section=projects&notice=project-deleted", http.StatusSeeOther)
}

func (server *Server) hopeSyncSkills(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if _, err := server.hope.SyncSkills(request.Context()); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(writer, request, "/?section=skills&notice=skills-synced", http.StatusSeeOther)
}

func (server *Server) hopeCreateSkill(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	skill := hope.Skill{ID: request.FormValue("id"), Name: request.FormValue("name"), Description: request.FormValue("description"), Keywords: controlplane.ParseKeywords(request.FormValue("keywords")), Role: request.FormValue("role"), Project: request.FormValue("project")}
	if _, err := server.hope.CreateSkill(request.Context(), skill, request.FormValue("body")); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?section=skills&notice=skill-created", http.StatusSeeOther)
}

func (server *Server) hopeUpdateSkill(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	skill := hope.Skill{
		ID: request.PathValue("skillID"), Name: request.FormValue("name"),
		Description: request.FormValue("description"), Keywords: controlplane.ParseKeywords(request.FormValue("keywords")),
		Role: request.FormValue("role"), Project: request.FormValue("project"),
	}
	if _, err := server.hope.UpdateSkill(request.Context(), skill, request.FormValue("body")); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?section=skills&skill="+skill.ID+"&notice=skill-saved", http.StatusSeeOther)
}

func (server *Server) hopeImportSkill(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	skill, err := server.hope.ImportSkill(request.Context(), request.FormValue("github_url"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?section=skills&skill="+skill.ID+"&notice=skill-imported", http.StatusSeeOther)
}

func (server *Server) hopeDeploySkill(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if err := server.hope.DeploySkill(request.Context(), request.PathValue("skillID")); err != nil {
		http.Error(writer, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(writer, request, "/?section=skills&notice=skill-deployed", http.StatusSeeOther)
}

func (server *Server) hopeRouteSkills(writer http.ResponseWriter, request *http.Request) {
	session, ok := server.requireHOPEGovernor(writer, request)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(request.FormValue("limit"))
	matches, err := server.hope.RouteSkills(request.Context(), hope.RouteRequest{Query: request.FormValue("query"), AgentID: request.FormValue("agent_id"), ProjectID: request.FormValue("project_id"), Limit: limit})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	matches, evidence := server.rankAmbiguousSkills(request.Context(), session.AgentID, request.FormValue("query"), matches)
	_ = dashboardTemplates.ExecuteTemplate(writer, "hope_route.html", struct {
		Query   string
		Matches []hope.SkillMatch
		Routing skillRouteEvidence
	}{Query: request.FormValue("query"), Matches: matches, Routing: evidence})
}

func (server *Server) hopeAutomationAction(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireHOPEGovernor(writer, request); !ok {
		return
	}
	if err := server.hope.AutomationAction(request.Context(), request.PathValue("jobID"), request.FormValue("action")); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(writer, request, "/?section=automations&notice=automation-updated", http.StatusSeeOther)
}

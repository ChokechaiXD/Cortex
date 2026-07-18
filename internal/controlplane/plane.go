package controlplane

import (
	"context"
	"fmt"
	"strings"

	"cortex.local/cortex/internal/automations"
	"cortex.local/cortex/internal/hermesruntime"
	"cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/integrationhub"
	"cortex.local/cortex/internal/projectcenter"
	"cortex.local/cortex/internal/skillcenter"
	"cortex.local/cortex/internal/workmodes"
)

type AgentStatus struct {
	Agent  hope.Agent
	Status integrationhub.Status
}

type Overview struct {
	Snapshot    hope.Snapshot
	Connections []integrationhub.Status
	Agents      []AgentStatus
	Jobs        []automations.Job
	JobsError   string
}

type Plane struct {
	store        *hope.Hub
	integrations *integrationhub.Hub
	modes        *workmodes.Manager
	projects     *projectcenter.Catalog
	skills       *skillcenter.Catalog
	automations  *automations.Manager
	hermes       HermesRuntime
}

type HermesRuntime interface {
	Gateways(context.Context) (map[string]hermesruntime.GatewayStatus, error)
	CreateProfile(context.Context, string) error
}

func New(
	store *hope.Hub,
	integrations *integrationhub.Hub,
	modes *workmodes.Manager,
	projects *projectcenter.Catalog,
	skills *skillcenter.Catalog,
	automations *automations.Manager,
	hermes HermesRuntime,
) *Plane {
	return &Plane{store: store, integrations: integrations, modes: modes, projects: projects, skills: skills, automations: automations, hermes: hermes}
}

func (plane *Plane) Overview(ctx context.Context, includeJobs bool) (Overview, error) {
	snapshot, err := plane.store.Snapshot(ctx)
	if err != nil {
		return Overview{}, err
	}
	overview := Overview{Snapshot: snapshot, Connections: plane.integrations.Snapshot(ctx)}
	gatewayStatuses, gatewayErr := plane.hermes.Gateways(ctx)
	for _, agent := range snapshot.Agents {
		status := integrationhub.Status{ID: "hermes", Name: "Hermes", State: integrationhub.StateStopped, Detail: "Gateway ยังไม่ทำงาน"}
		if gatewayErr != nil {
			status.State, status.Detail = integrationhub.StateDegraded, gatewayErr.Error()
		} else if gateway, exists := gatewayStatuses[agent.Profile]; exists && gateway.Running {
			status.State, status.PID, status.Detail = integrationhub.StateExternal, gateway.PID, "Gateway ทำงานอยู่ภายนอก HOPE"
			if managed, owned, _ := plane.store.ManagedProcess(ctx, "hermes:"+agent.Profile); owned && managed.PID > 0 && managed.PID == gateway.PID && hermesruntime.ProcessMatches(managed.PID, managed.StartedAt) {
				status.State, status.Managed, status.Detail = integrationhub.StateRunning, true, "HOPE เป็นผู้เปิด gateway นี้"
			}
		}
		overview.Agents = append(overview.Agents, AgentStatus{Agent: agent, Status: status})
	}
	if includeJobs {
		overview.Jobs, err = plane.automations.List(ctx)
		if err != nil {
			overview.JobsError = err.Error()
		}
	}
	return overview, nil
}

func (plane *Plane) WorkModes(ctx context.Context) ([]hope.WorkMode, error) {
	return plane.store.WorkModes(ctx)
}

func (plane *Plane) Agents(ctx context.Context) ([]hope.Agent, error) {
	return plane.store.Agents(ctx)
}

func (plane *Plane) Agent(ctx context.Context, id string) (hope.Agent, error) {
	return plane.store.Agent(ctx, id)
}

func (plane *Plane) AgentStatuses(ctx context.Context) ([]AgentStatus, error) {
	agents, err := plane.store.Agents(ctx)
	if err != nil {
		return nil, err
	}
	gatewayStatuses, gatewayErr := plane.hermes.Gateways(ctx)
	result := make([]AgentStatus, 0, len(agents))
	for _, agent := range agents {
		status := integrationhub.Status{ID: "hermes", Name: "Hermes", State: integrationhub.StateStopped, Detail: "Gateway is not running"}
		if gatewayErr != nil {
			status.State, status.Detail = integrationhub.StateDegraded, gatewayErr.Error()
		} else if gateway, exists := gatewayStatuses[agent.Profile]; exists && gateway.Running {
			status.State, status.PID, status.Detail = integrationhub.StateExternal, gateway.PID, "Running"
			if managed, owned, _ := plane.store.ManagedProcess(ctx, "hermes:"+agent.Profile); owned && managed.PID > 0 && managed.PID == gateway.PID && hermesruntime.ProcessMatches(managed.PID, managed.StartedAt) {
				status.State, status.Managed, status.Detail = integrationhub.StateRunning, true, "Running"
			}
		}
		result = append(result, AgentStatus{Agent: agent, Status: status})
	}
	return result, nil
}

func (plane *Plane) Projects(ctx context.Context) ([]hope.Project, error) {
	return plane.store.Projects(ctx)
}

func (plane *Plane) ProjectRoots(ctx context.Context) ([]string, error) {
	return plane.store.ProjectRoots(ctx)
}

func (plane *Plane) Skills(ctx context.Context) ([]hope.Skill, error) {
	return plane.store.Skills(ctx)
}

func (plane *Plane) RecentEvents(ctx context.Context, limit int) ([]hope.ActionEvent, error) {
	return plane.store.RecentEvents(ctx, limit)
}

func (plane *Plane) Connections(ctx context.Context) []integrationhub.Status {
	return plane.integrations.SnapshotExcluding(ctx, "hermes", "telegram")
}

func (plane *Plane) Automations(ctx context.Context) ([]automations.Job, error) {
	return plane.automations.List(ctx)
}

func (plane *Plane) IntegrationAction(ctx context.Context, request integrationhub.ActionRequest) integrationhub.ActionResult {
	return plane.integrations.Execute(ctx, request)
}

func (plane *Plane) WorkMode(ctx context.Context, id, action string) (workmodes.Result, error) {
	return plane.modes.Execute(ctx, id, action)
}

func (plane *Plane) SaveWorkMode(ctx context.Context, mode hope.WorkMode) error {
	return plane.store.SaveWorkMode(ctx, mode)
}

func (plane *Plane) SaveAgent(ctx context.Context, agent hope.Agent, createProfile bool) error {
	if createProfile {
		if err := plane.hermes.CreateProfile(ctx, agent.Profile); err != nil {
			return err
		}
	}
	return plane.store.SaveAgent(ctx, agent)
}

func (plane *Plane) SaveProject(ctx context.Context, project hope.Project) error {
	return plane.store.SaveProject(ctx, project)
}

func (plane *Plane) DeleteProject(ctx context.Context, id string) error {
	return plane.store.DeleteProject(ctx, id)
}

func (plane *Plane) AddProjectRoot(ctx context.Context, path string) error {
	return plane.store.AddProjectRoot(ctx, path)
}

func (plane *Plane) DiscoverProjects(ctx context.Context) ([]hope.Project, error) {
	return plane.projects.Discover(ctx)
}

func (plane *Plane) OpenProject(ctx context.Context, id string) error {
	return plane.projects.Open(ctx, id)
}

func (plane *Plane) SyncSkills(ctx context.Context) (int, error) {
	return plane.skills.Sync(ctx)
}

func (plane *Plane) CreateSkill(ctx context.Context, skill hope.Skill, body string) (hope.Skill, error) {
	return plane.skills.Create(ctx, skill, body)
}

func (plane *Plane) ReadSkill(ctx context.Context, id string) (hope.Skill, string, error) {
	return plane.skills.Read(ctx, id)
}

func (plane *Plane) UpdateSkill(ctx context.Context, skill hope.Skill, body string) (hope.Skill, error) {
	return plane.skills.Update(ctx, skill, body)
}

func (plane *Plane) ImportSkill(ctx context.Context, url string) (hope.Skill, error) {
	return plane.skills.ImportGitHub(ctx, url)
}

func (plane *Plane) DeploySkill(ctx context.Context, id string) error {
	return plane.skills.Deploy(ctx, id)
}

func (plane *Plane) RouteSkills(ctx context.Context, request hope.RouteRequest) ([]hope.SkillMatch, error) {
	return plane.store.RouteSkills(ctx, request)
}

func (plane *Plane) SkillOutcome(ctx context.Context, id, outcome string) error {
	return plane.store.RecordSkillOutcome(ctx, id, outcome)
}

func (plane *Plane) SaveSkillRoute(ctx context.Context, pack hope.ContextPack) (string, error) {
	return plane.store.SaveSkillRoute(ctx, pack)
}

func (plane *Plane) ContextSkillFeedback(ctx context.Context, feedback hope.SkillFeedback) error {
	return plane.store.ApplySkillFeedback(ctx, feedback)
}

func (plane *Plane) AutomationAction(ctx context.Context, id, action string) error {
	if err := plane.automations.Execute(ctx, id, action); err != nil {
		_ = plane.store.RecordAction(ctx, hope.ActionEvent{Target: "cron:" + id, Action: action, Status: "error", Message: err.Error()})
		return err
	}
	return plane.store.RecordAction(ctx, hope.ActionEvent{Target: "cron:" + id, Action: action, Status: "ok", Message: "Hermes cron " + action + " completed"})
}

func ParseKeywords(value string) []string {
	seen := map[string]bool{}
	var result []string
	for _, keyword := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" && !seen[keyword] {
			seen[keyword] = true
			result = append(result, keyword)
		}
	}
	return result
}

func ValidateAction(value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("unsupported action")
}

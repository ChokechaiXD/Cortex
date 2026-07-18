package hope

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (hub *Hub) Snapshot(ctx context.Context) (Snapshot, error) {
	agents, err := hub.Agents(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	modes, err := hub.WorkModes(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	projects, err := hub.Projects(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	roots, err := hub.ProjectRoots(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	skills, err := hub.Skills(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	events, err := hub.RecentEvents(ctx, 20)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Agents: agents, Modes: modes, Projects: projects, Roots: roots, Skills: skills, Events: events}, nil
}

func (hub *Hub) Agents(ctx context.Context) ([]Agent, error) {
	rows, err := hub.db.QueryContext(ctx, `SELECT id,name,role,profile,telegram_url,avatar_path,summary,capabilities_json,persona_path,persona_note,enabled FROM agents ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list HOPE agents: %w", err)
	}
	defer rows.Close()
	var result []Agent
	for rows.Next() {
		var agent Agent
		var capabilities string
		var enabled int
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Role, &agent.Profile, &agent.TelegramURL, &agent.AvatarPath, &agent.Summary, &capabilities, &agent.PersonaPath, &agent.PersonaNote, &enabled); err != nil {
			return nil, fmt.Errorf("scan HOPE agent: %w", err)
		}
		agent.Capabilities = decodeStrings(capabilities)
		agent.Enabled = enabled == 1
		result = append(result, agent)
	}
	return result, rows.Err()
}

func (hub *Hub) Agent(ctx context.Context, id string) (Agent, error) {
	var agent Agent
	var capabilities string
	var enabled int
	err := hub.db.QueryRowContext(ctx, `SELECT id,name,role,profile,telegram_url,avatar_path,summary,capabilities_json,persona_path,persona_note,enabled FROM agents WHERE id=?`, strings.TrimSpace(id)).
		Scan(&agent.ID, &agent.Name, &agent.Role, &agent.Profile, &agent.TelegramURL, &agent.AvatarPath, &agent.Summary, &capabilities, &agent.PersonaPath, &agent.PersonaNote, &enabled)
	if err != nil {
		return Agent{}, fmt.Errorf("find HOPE agent: %w", err)
	}
	agent.Capabilities = decodeStrings(capabilities)
	agent.Enabled = enabled == 1
	return agent, nil
}

func (hub *Hub) SaveAgent(ctx context.Context, agent Agent) error {
	agent.ID = slug(agent.ID)
	agent.Profile = strings.TrimSpace(agent.Profile)
	agent.Name = strings.TrimSpace(agent.Name)
	agent.Role = strings.TrimSpace(agent.Role)
	agent.Capabilities = uniqueStrings(agent.Capabilities)
	if agent.ID == "" || agent.Profile == "" || agent.Name == "" {
		return fmt.Errorf("agent id, name and Hermes profile are required")
	}
	capabilities, _ := encodeStrings(agent.Capabilities)
	_, err := hub.db.ExecContext(ctx, `INSERT INTO agents(id,name,role,profile,telegram_url,avatar_path,summary,capabilities_json,persona_path,persona_note,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,role=excluded.role,profile=excluded.profile,telegram_url=excluded.telegram_url,avatar_path=excluded.avatar_path,summary=excluded.summary,capabilities_json=excluded.capabilities_json,persona_path=excluded.persona_path,persona_note=excluded.persona_note,enabled=excluded.enabled`,
		agent.ID, agent.Name, agent.Role, agent.Profile, strings.TrimSpace(agent.TelegramURL), strings.TrimSpace(agent.AvatarPath), strings.TrimSpace(agent.Summary), capabilities, strings.TrimSpace(agent.PersonaPath), strings.TrimSpace(agent.PersonaNote), boolInt(agent.Enabled))
	if err != nil {
		return fmt.Errorf("save HOPE agent: %w", err)
	}
	return nil
}

func (hub *Hub) WorkModes(ctx context.Context) ([]WorkMode, error) {
	rows, err := hub.db.QueryContext(ctx, `SELECT id,name,description,integrations_json,agents_json,open_telegram FROM work_modes ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list HOPE work modes: %w", err)
	}
	defer rows.Close()
	var result []WorkMode
	for rows.Next() {
		var mode WorkMode
		var integrations, agents string
		var openTelegram int
		if err := rows.Scan(&mode.ID, &mode.Name, &mode.Description, &integrations, &agents, &openTelegram); err != nil {
			return nil, fmt.Errorf("scan HOPE work mode: %w", err)
		}
		mode.Integrations = decodeStrings(integrations)
		mode.Agents = decodeStrings(agents)
		mode.OpenTelegram = openTelegram == 1
		result = append(result, mode)
	}
	return result, rows.Err()
}

func (hub *Hub) WorkMode(ctx context.Context, id string) (WorkMode, error) {
	var mode WorkMode
	var integrations, agents string
	var openTelegram int
	err := hub.db.QueryRowContext(ctx, `SELECT id,name,description,integrations_json,agents_json,open_telegram FROM work_modes WHERE id=?`, strings.TrimSpace(id)).
		Scan(&mode.ID, &mode.Name, &mode.Description, &integrations, &agents, &openTelegram)
	if err != nil {
		return WorkMode{}, fmt.Errorf("find HOPE work mode: %w", err)
	}
	mode.Integrations = decodeStrings(integrations)
	mode.Agents = decodeStrings(agents)
	mode.OpenTelegram = openTelegram == 1
	return mode, nil
}

func (hub *Hub) SaveWorkMode(ctx context.Context, mode WorkMode) error {
	mode.ID = slug(mode.ID)
	mode.Name = strings.TrimSpace(mode.Name)
	mode.Description = strings.TrimSpace(mode.Description)
	mode.Integrations = uniqueStrings(mode.Integrations)
	mode.Agents = uniqueStrings(mode.Agents)
	if mode.ID == "" || mode.Name == "" {
		return fmt.Errorf("work mode id and name are required")
	}
	integrations, _ := encodeStrings(mode.Integrations)
	agents, _ := encodeStrings(mode.Agents)
	_, err := hub.db.ExecContext(ctx, `INSERT INTO work_modes(id,name,description,integrations_json,agents_json,open_telegram) VALUES(?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,integrations_json=excluded.integrations_json,agents_json=excluded.agents_json,open_telegram=excluded.open_telegram`,
		mode.ID, mode.Name, mode.Description, integrations, agents, boolInt(mode.OpenTelegram))
	if err != nil {
		return fmt.Errorf("save work mode: %w", err)
	}
	return nil
}

func (hub *Hub) ProjectRoots(ctx context.Context) ([]string, error) {
	rows, err := hub.db.QueryContext(ctx, `SELECT path FROM project_roots ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("list project roots: %w", err)
	}
	defer rows.Close()
	var roots []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

func (hub *Hub) AddProjectRoot(ctx context.Context, root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("project root is not an accessible folder")
	}
	_, err = hub.db.ExecContext(ctx, `INSERT OR IGNORE INTO project_roots(path) VALUES(?)`, root)
	return err
}

func (hub *Hub) Projects(ctx context.Context) ([]Project, error) {
	rows, err := hub.db.QueryContext(ctx, `SELECT id,name,path,kind,description,goal,status,progress,current_state,next_action,active,updated_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	var result []Project
	for rows.Next() {
		var project Project
		var active int
		var updated string
		if err := rows.Scan(&project.ID, &project.Name, &project.Path, &project.Kind, &project.Description, &project.Goal, &project.Status, &project.Progress, &project.CurrentState, &project.NextAction, &active, &updated); err != nil {
			return nil, err
		}
		project.Active = active == 1
		project.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		if project.Path != "" {
			_, err := os.Stat(project.Path)
			project.Available = err == nil
		}
		result = append(result, project)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range result {
		result[index].AgentIDs, _ = hub.projectAgents(ctx, result[index].ID)
	}
	return result, nil
}

func (hub *Hub) Project(ctx context.Context, id string) (Project, error) {
	var project Project
	var active int
	var updated string
	err := hub.db.QueryRowContext(ctx, `SELECT id,name,path,kind,description,goal,status,progress,current_state,next_action,active,updated_at FROM projects WHERE id=?`, strings.TrimSpace(id)).
		Scan(&project.ID, &project.Name, &project.Path, &project.Kind, &project.Description, &project.Goal, &project.Status, &project.Progress, &project.CurrentState, &project.NextAction, &active, &updated)
	if err != nil {
		return Project{}, fmt.Errorf("find project: %w", err)
	}
	project.Active = active == 1
	project.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if project.Path != "" {
		_, err = os.Stat(project.Path)
		project.Available = err == nil
	}
	project.AgentIDs, _ = hub.projectAgents(ctx, project.ID)
	return project, nil
}

func (hub *Hub) SaveProject(ctx context.Context, project Project) error {
	project.Path = strings.TrimSpace(project.Path)
	if project.Path != "" {
		project.Path = filepath.Clean(project.Path)
	}
	project.Name = strings.TrimSpace(project.Name)
	if project.ID == "" {
		if project.Path != "" {
			project.ID = slug(filepath.Base(project.Path))
		} else {
			project.ID = slug(project.Name)
		}
	}
	if project.ID == "" {
		project.ID = "project-" + projectPathSuffix(project.Path)
	}
	if project.Path != "" {
		var existingID string
		err := hub.db.QueryRowContext(ctx, `SELECT id FROM projects WHERE path=?`, project.Path).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("check project path: %w", err)
		}
		if err == nil {
			project.ID = existingID
		} else {
			var occupiedPath string
			err := hub.db.QueryRowContext(ctx, `SELECT path FROM projects WHERE id=?`, project.ID).Scan(&occupiedPath)
			if err != nil && err != sql.ErrNoRows {
				return fmt.Errorf("check project id: %w", err)
			}
			if err == nil && !strings.EqualFold(filepath.Clean(occupiedPath), project.Path) {
				project.ID += "-" + projectPathSuffix(project.Path)
			}
		}
	}
	if project.Name == "" {
		project.Name = filepath.Base(project.Path)
	}
	if project.Kind == "" {
		project.Kind = "workspace"
	}
	if project.Status == "" {
		project.Status = "active"
	}
	if project.Progress < 0 || project.Progress > 100 {
		return fmt.Errorf("project progress must be between 0 and 100")
	}
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO projects(id,name,path,kind,description,goal,status,progress,current_state,next_action,active,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,path=excluded.path,kind=excluded.kind,description=excluded.description,goal=excluded.goal,status=excluded.status,progress=excluded.progress,current_state=excluded.current_state,next_action=excluded.next_action,active=excluded.active,updated_at=excluded.updated_at`,
		project.ID, project.Name, project.Path, project.Kind, strings.TrimSpace(project.Description), strings.TrimSpace(project.Goal), strings.TrimSpace(project.Status), project.Progress, strings.TrimSpace(project.CurrentState), strings.TrimSpace(project.NextAction), boolInt(project.Active), nowText())
	if err != nil {
		return fmt.Errorf("save project: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_agents WHERE project_id=?`, project.ID); err != nil {
		return err
	}
	for _, agentID := range uniqueStrings(project.AgentIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO project_agents(project_id,agent_id) VALUES(?,?)`, project.ID, agentID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (hub *Hub) DeleteProject(ctx context.Context, id string) error {
	_, err := hub.db.ExecContext(ctx, `DELETE FROM projects WHERE id=?`, strings.TrimSpace(id))
	return err
}

func (hub *Hub) projectAgents(ctx context.Context, projectID string) ([]string, error) {
	rows, err := hub.db.QueryContext(ctx, `SELECT agent_id FROM project_agents WHERE project_id=? ORDER BY agent_id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return nil, err
		}
		result = append(result, agentID)
	}
	return result, rows.Err()
}

func projectPathSuffix(path string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	return fmt.Sprintf("%x", sum[:4])
}

func (hub *Hub) Skills(ctx context.Context) ([]Skill, error) {
	rows, err := hub.db.QueryContext(ctx, `SELECT id,name,description,path,source,source_url,keywords_json,role,project,enabled,use_count,success_count,failure_count,updated_at FROM skills ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	var result []Skill
	for rows.Next() {
		var item Skill
		var keywords, updated string
		var enabled int
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Path, &item.Source, &item.SourceURL, &keywords, &item.Role, &item.Project, &enabled, &item.UseCount, &item.SuccessCount, &item.FailureCount, &updated); err != nil {
			return nil, err
		}
		item.Keywords = decodeStrings(keywords)
		item.Enabled = enabled == 1
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (hub *Hub) SaveSkill(ctx context.Context, skill Skill) error {
	skill.ID = slug(skill.ID)
	if skill.ID == "" {
		skill.ID = slug(skill.Name)
	}
	if skill.ID == "" || strings.TrimSpace(skill.Name) == "" {
		return fmt.Errorf("skill id and name are required")
	}
	keywords, _ := encodeStrings(skill.Keywords)
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO skills(id,name,description,path,source,source_url,keywords_json,role,project,enabled,use_count,success_count,failure_count,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,path=excluded.path,source=excluded.source,source_url=excluded.source_url,keywords_json=excluded.keywords_json,role=excluded.role,project=excluded.project,enabled=excluded.enabled,use_count=excluded.use_count,success_count=excluded.success_count,failure_count=excluded.failure_count,updated_at=excluded.updated_at`,
		skill.ID, strings.TrimSpace(skill.Name), strings.TrimSpace(skill.Description), skill.Path, skill.Source, strings.TrimSpace(skill.SourceURL), keywords,
		skill.Role, skill.Project, boolInt(skill.Enabled), skill.UseCount, skill.SuccessCount, skill.FailureCount, nowText())
	if err != nil {
		return fmt.Errorf("save skill: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM skill_fts WHERE skill_id=?`, skill.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO skill_fts(skill_id,name,description,keywords) VALUES(?,?,?,?)`,
		skill.ID, skill.Name, skill.Description, strings.Join(skill.Keywords, " ")); err != nil {
		return err
	}
	return tx.Commit()
}

func (hub *Hub) ManagedProcess(ctx context.Context, key string) (ManagedProcess, bool, error) {
	var item ManagedProcess
	var started string
	err := hub.db.QueryRowContext(ctx, `SELECT process_key,pid,command,started_at FROM managed_processes WHERE process_key=?`, key).
		Scan(&item.Key, &item.PID, &item.Command, &started)
	if err == sql.ErrNoRows {
		return ManagedProcess{}, false, nil
	}
	if err != nil {
		return ManagedProcess{}, false, err
	}
	item.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	return item, true, nil
}

func (hub *Hub) SaveManagedProcess(ctx context.Context, item ManagedProcess) error {
	_, err := hub.db.ExecContext(ctx, `INSERT INTO managed_processes(process_key,pid,command,started_at) VALUES(?,?,?,?)
ON CONFLICT(process_key) DO UPDATE SET pid=excluded.pid,command=excluded.command,started_at=excluded.started_at`,
		item.Key, item.PID, item.Command, item.StartedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (hub *Hub) DeleteManagedProcess(ctx context.Context, key string) error {
	_, err := hub.db.ExecContext(ctx, `DELETE FROM managed_processes WHERE process_key=?`, key)
	return err
}

func (hub *Hub) RecordAction(ctx context.Context, event ActionEvent) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := hub.db.ExecContext(ctx, `INSERT INTO action_events(id,target,action,status,message,created_at) VALUES(?,?,?,?,?,?)`,
		event.ID, event.Target, event.Action, event.Status, event.Message, event.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (hub *Hub) RecentEvents(ctx context.Context, limit int) ([]ActionEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 20
	}
	rows, err := hub.db.QueryContext(ctx, `SELECT id,target,action,status,message,created_at FROM action_events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ActionEvent
	for rows.Next() {
		var event ActionEvent
		var created string
		if err := rows.Scan(&event.ID, &event.Target, &event.Action, &event.Status, &event.Message, &created); err != nil {
			return nil, err
		}
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, event)
	}
	return result, rows.Err()
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	lastDash := false
	for _, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if valid {
			out.WriteRune(r)
			lastDash = false
		} else if !lastDash && out.Len() > 0 {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func SortSkills(skills []Skill) {
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

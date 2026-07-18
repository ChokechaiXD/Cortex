package hope

import "time"

type Agent struct {
	ID           string
	Name         string
	Role         string
	Profile      string
	TelegramURL  string
	AvatarPath   string
	Summary      string
	Capabilities []string
	PersonaPath  string
	PersonaNote  string
	Enabled      bool
}

type WorkMode struct {
	ID           string
	Name         string
	Description  string
	Integrations []string
	Agents       []string
	OpenTelegram bool
}

type Project struct {
	ID           string
	Name         string
	Path         string
	Kind         string
	Description  string
	Goal         string
	Status       string
	Progress     int
	CurrentState string
	NextAction    string
	AgentIDs      []string
	Available     bool
	Active        bool
	UpdatedAt     time.Time
}

type Skill struct {
	ID           string
	Name         string
	Description  string
	Path         string
	Source       string
	SourceURL    string
	Keywords     []string
	Role         string
	Project      string
	Enabled      bool
	UseCount     int
	SuccessCount int
	FailureCount int
	UpdatedAt    time.Time
}

type ManagedProcess struct {
	Key       string
	PID       int
	Command   string
	StartedAt time.Time
}

type ActionEvent struct {
	ID        string
	Target    string
	Action    string
	Status    string
	Message   string
	CreatedAt time.Time
}

type Snapshot struct {
	Agents   []Agent
	Modes    []WorkMode
	Projects []Project
	Roots    []string
	Skills   []Skill
	Events   []ActionEvent
}

type RouteRequest struct {
	Query     string
	AgentID   string
	ProjectID string
	Limit     int
}

type SkillMatch struct {
	Skill  Skill
	Score  float64
	Reason string
}

type ContextPack struct {
	ID             string
	IdempotencyKey string
	AgentID        string
	SessionID      string
	Query          string
	ProjectID      string
	Router         string
	InputTokens    int
	OutputTokens   int
	Skills         []SkillMatch
}

type SkillFeedback struct {
	IdempotencyKey string
	PackID         string
	SkillID        string
	AgentID        string
	Outcome        string
}

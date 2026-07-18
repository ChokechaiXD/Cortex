package integrationhub

import "context"

type State string

const (
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateExternal State = "external"
	StateMissing  State = "missing"
	StateDegraded State = "degraded"
	StateConflict State = "conflict"
)

type Status struct {
	ID      string
	Name    string
	State   State
	Managed bool
	Detail  string
	URL     string
	PID     int
}

type ActionRequest struct {
	Integration string
	Action      string
	Target      string
}

type ActionResult struct {
	Integration string
	Action      string
	Target      string
	Status      Status
	Message     string
	OpenURL     string
	Err         error
}

type Adapter interface {
	ID() string
	Probe(context.Context, string) Status
	Execute(context.Context, ActionRequest) ActionResult
}

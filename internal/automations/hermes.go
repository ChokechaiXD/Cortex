package automations

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cortex.local/cortex/internal/hermesruntime"
)

var jobHeader = regexp.MustCompile(`^([a-f0-9]{12}) \[([^]]+)\]$`)

type Job struct {
	ID       string
	Name     string
	Status   string
	Schedule string
	NextRun  string
	LastRun  string
	Deliver  string
}

type Manager struct {
	client *hermesruntime.Client
}

func New(client *hermesruntime.Client) *Manager { return &Manager{client: client} }

func (manager *Manager) List(ctx context.Context) ([]Job, error) {
	listCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	output, err := manager.client.Run(listCtx, "", "cron", "list")
	if err != nil {
		return nil, err
	}
	return parseJobs(output), nil
}

func (manager *Manager) Execute(ctx context.Context, id, action string) error {
	if !jobHeader.MatchString(id + " [active]") {
		return fmt.Errorf("invalid cron job id")
	}
	switch action {
	case "run", "pause", "resume":
	default:
		return fmt.Errorf("unsupported cron action")
	}
	actionCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	_, err := manager.client.Run(actionCtx, "", "cron", action, id)
	return err
}

func parseJobs(output string) []Job {
	var jobs []Job
	var current *Job
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if match := jobHeader.FindStringSubmatch(line); len(match) == 3 {
			jobs = append(jobs, Job{ID: match[1], Status: match[2]})
			current = &jobs[len(jobs)-1]
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Name":
			current.Name = value
		case "Schedule":
			current.Schedule = value
		case "Next run":
			current.NextRun = value
		case "Last run":
			current.LastRun = value
		case "Deliver":
			current.Deliver = value
		}
	}
	return jobs
}

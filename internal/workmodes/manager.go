package workmodes

import (
	"context"
	"fmt"
	"sync"

	"cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/integrationhub"
)

type Result struct {
	Mode     hope.WorkMode
	Steps    []integrationhub.ActionResult
	OpenURLs []string
}

type Manager struct {
	store        *hope.Hub
	integrations *integrationhub.Hub
}

func New(store *hope.Hub, integrations *integrationhub.Hub) *Manager {
	return &Manager{store: store, integrations: integrations}
}

func (manager *Manager) Execute(ctx context.Context, modeID, action string) (Result, error) {
	mode, err := manager.store.WorkMode(ctx, modeID)
	if err != nil {
		return Result{}, err
	}
	if action != "start" && action != "stop" {
		return Result{}, fmt.Errorf("work mode action must be start or stop")
	}
	result := Result{Mode: mode}
	if action == "start" {
		requests := make([]integrationhub.ActionRequest, 0, len(mode.Integrations))
		for _, integrationID := range mode.Integrations {
			requests = append(requests, integrationhub.ActionRequest{Integration: integrationID, Action: "start"})
		}
		result.Steps = append(result.Steps, manager.executeConcurrent(ctx, requests)...)
		requests = requests[:0]
		for _, agentID := range mode.Agents {
			agent, findErr := manager.store.Agent(ctx, agentID)
			if findErr != nil {
				result.Steps = append(result.Steps, integrationhub.ActionResult{Integration: "hermes", Action: "start", Target: agentID, Err: findErr})
				continue
			}
			requests = append(requests, integrationhub.ActionRequest{Integration: "hermes", Action: "start", Target: agent.Profile})
			if mode.OpenTelegram && agent.TelegramURL != "" {
				result.OpenURLs = append(result.OpenURLs, agent.TelegramURL)
			}
		}
		result.Steps = append(result.Steps, manager.executeConcurrent(ctx, requests)...)
		return result, nil
	}
	requests := make([]integrationhub.ActionRequest, 0, len(mode.Agents))
	for index := len(mode.Agents) - 1; index >= 0; index-- {
		agent, findErr := manager.store.Agent(ctx, mode.Agents[index])
		if findErr != nil {
			continue
		}
		requests = append(requests, integrationhub.ActionRequest{Integration: "hermes", Action: "stop", Target: agent.Profile})
	}
	result.Steps = append(result.Steps, manager.executeConcurrent(ctx, requests)...)
	requests = requests[:0]
	for index := len(mode.Integrations) - 1; index >= 0; index-- {
		requests = append(requests, integrationhub.ActionRequest{Integration: mode.Integrations[index], Action: "stop"})
	}
	result.Steps = append(result.Steps, manager.executeConcurrent(ctx, requests)...)
	return result, nil
}

func (manager *Manager) executeConcurrent(ctx context.Context, requests []integrationhub.ActionRequest) []integrationhub.ActionResult {
	results := make([]integrationhub.ActionResult, len(requests))
	var wait sync.WaitGroup
	for index, request := range requests {
		index, request := index, request
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = manager.integrations.Execute(ctx, request)
		}()
	}
	wait.Wait()
	return results
}

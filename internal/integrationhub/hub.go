package integrationhub

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cortex.local/cortex/internal/hope"
)

type Hub struct {
	store    *hope.Hub
	adapters map[string]Adapter
	locksMu  sync.Mutex
	locks    map[string]*sync.Mutex
}

func New(store *hope.Hub, adapters ...Adapter) *Hub {
	hub := &Hub{store: store, adapters: make(map[string]Adapter), locks: make(map[string]*sync.Mutex)}
	for _, adapter := range adapters {
		if adapter != nil {
			hub.adapters[adapter.ID()] = adapter
		}
	}
	return hub
}

func (hub *Hub) Snapshot(ctx context.Context) []Status {
	return hub.SnapshotExcluding(ctx)
}

func (hub *Hub) SnapshotExcluding(ctx context.Context, excluded ...string) []Status {
	skip := map[string]bool{}
	for _, id := range excluded {
		skip[id] = true
	}
	adapters := make([]Adapter, 0, len(hub.adapters))
	for id, adapter := range hub.adapters {
		if !skip[id] {
			adapters = append(adapters, adapter)
		}
	}
	result := make([]Status, len(adapters))
	var wait sync.WaitGroup
	for index, adapter := range adapters {
		currentIndex, currentAdapter := index, adapter
		wait.Add(1)
		go func() {
			defer wait.Done()
			result[currentIndex] = currentAdapter.Probe(ctx, "")
		}()
	}
	wait.Wait()
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (hub *Hub) Probe(ctx context.Context, integration, target string) Status {
	adapter, ok := hub.adapters[integration]
	if !ok {
		return Status{ID: integration, Name: integration, State: StateMissing, Detail: "ยังไม่ได้ติดตั้ง connector"}
	}
	return adapter.Probe(ctx, target)
}

func (hub *Hub) Execute(ctx context.Context, request ActionRequest) ActionResult {
	request.Integration = strings.TrimSpace(request.Integration)
	request.Action = strings.TrimSpace(request.Action)
	request.Target = strings.TrimSpace(request.Target)
	adapter, ok := hub.adapters[request.Integration]
	if !ok {
		return ActionResult{Integration: request.Integration, Action: request.Action, Target: request.Target, Err: fmt.Errorf("integration is unavailable")}
	}
	lock := hub.lock(request.Integration + ":" + request.Target)
	lock.Lock()
	defer lock.Unlock()
	result := adapter.Execute(ctx, request)
	status := "ok"
	message := result.Message
	if result.Err != nil {
		status = "error"
		message = result.Err.Error()
	}
	_ = hub.store.RecordAction(ctx, hope.ActionEvent{
		Target: request.Integration + targetSuffix(request.Target), Action: request.Action,
		Status: status, Message: message, CreatedAt: time.Now().UTC(),
	})
	return result
}

func (hub *Hub) lock(key string) *sync.Mutex {
	hub.locksMu.Lock()
	defer hub.locksMu.Unlock()
	if hub.locks[key] == nil {
		hub.locks[key] = &sync.Mutex{}
	}
	return hub.locks[key]
}

func targetSuffix(target string) string {
	if target == "" {
		return ""
	}
	return ":" + target
}

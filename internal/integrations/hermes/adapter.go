package hermes

import (
	"context"
	"fmt"
	"time"

	"cortex.local/cortex/internal/hermesruntime"
	"cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/integrationhub"
)

type Adapter struct {
	store  *hope.Hub
	client *hermesruntime.Client
}

func New(store *hope.Hub, client *hermesruntime.Client) *Adapter {
	return &Adapter{store: store, client: client}
}

func (*Adapter) ID() string { return "hermes" }

func (adapter *Adapter) Probe(ctx context.Context, target string) integrationhub.Status {
	profile := target
	if profile == "" {
		profile = "default"
	}
	gateway := adapter.client.Gateway(ctx, profile)
	status := integrationhub.Status{ID: adapter.ID(), Name: "Hermes", State: integrationhub.StateStopped, Detail: gateway.Detail, PID: gateway.PID}
	if !gateway.Installed {
		status.State = integrationhub.StateMissing
		return status
	}
	if !gateway.Running {
		return status
	}
	key := "hermes:" + profile
	managed, owned, _ := adapter.store.ManagedProcess(ctx, key)
	status.State = integrationhub.StateExternal
	status.Detail = "Gateway ทำงานอยู่ภายนอก HOPE"
	if owned && managed.PID > 0 && managed.PID == gateway.PID && hermesruntime.ProcessMatches(managed.PID, managed.StartedAt) {
		status.State = integrationhub.StateRunning
		status.Managed = true
		status.Detail = "HOPE เป็นผู้เปิด gateway นี้"
	}
	return status
}

func (adapter *Adapter) Execute(ctx context.Context, request integrationhub.ActionRequest) integrationhub.ActionResult {
	profile := request.Target
	if profile == "" {
		profile = "default"
	}
	result := integrationhub.ActionResult{Integration: adapter.ID(), Action: request.Action, Target: profile}
	switch request.Action {
	case "start":
		current := adapter.Probe(ctx, profile)
		if current.State == integrationhub.StateRunning || current.State == integrationhub.StateExternal {
			result.Status = current
			result.Message = "Hermes gateway ทำงานอยู่แล้ว"
			return result
		}
		gateway, err := adapter.client.GatewayAction(ctx, profile, "start")
		if err != nil {
			result.Status = adapter.Probe(ctx, profile)
			result.Err = err
			return result
		}
		if gateway.PID > 0 {
			_ = adapter.store.SaveManagedProcess(ctx, hope.ManagedProcess{
				Key: "hermes:" + profile, PID: gateway.PID, Command: "hermes -p " + profile + " gateway start", StartedAt: time.Now().UTC(),
			})
		}
		result.Status = adapter.Probe(ctx, profile)
		result.Message = "เปิด Hermes gateway แล้ว"
		return result
	case "stop":
		key := "hermes:" + profile
		managed, owned, err := adapter.store.ManagedProcess(ctx, key)
		if err != nil {
			result.Err = err
			return result
		}
		current := adapter.client.Gateway(ctx, profile)
		if !owned || managed.PID <= 0 || managed.PID != current.PID || !hermesruntime.ProcessMatches(managed.PID, managed.StartedAt) {
			result.Status = adapter.Probe(ctx, profile)
			result.Err = fmt.Errorf("HOPE will not stop a Hermes gateway it does not own")
			return result
		}
		if _, err := adapter.client.GatewayAction(ctx, profile, "stop"); err != nil {
			result.Err = err
			return result
		}
		_ = adapter.store.DeleteManagedProcess(ctx, key)
		result.Status = adapter.Probe(ctx, profile)
		result.Message = "ปิด Hermes gateway แล้ว"
		return result
	default:
		result.Err = fmt.Errorf("unsupported Hermes action %q", request.Action)
		return result
	}
}

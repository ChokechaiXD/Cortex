package controlcenter

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestRuntimeReportsCurrentProcessAndQueuesOneValidatedAction(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime("0.2.0", "127.0.0.1:7777", `C:\Cortex`)
	status, err := runtime.Status(context.Background())
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}
	if !status.Running || status.PID != os.Getpid() || status.Version != "0.2.0" ||
		status.Listen != "127.0.0.1:7777" || status.Port != 7777 {
		t.Fatalf("runtime status = %#v", status)
	}
	if err := runtime.Request(Action("invalid")); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("invalid action error = %v", err)
	}
	if err := runtime.Request(ActionRestart); err != nil {
		t.Fatalf("request restart: %v", err)
	}
	if err := runtime.Request(ActionStop); !errors.Is(err, ErrActionPending) {
		t.Fatalf("second action error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	action, err := runtime.Next(ctx)
	if err != nil {
		t.Fatalf("receive runtime action: %v", err)
	}
	if action != ActionRestart {
		t.Fatalf("queued action = %q", action)
	}
	status, err = runtime.Status(context.Background())
	if err != nil || status.Pending != ActionRestart {
		t.Fatalf("pending action cleared before restart was ready: status=%#v err=%v", status, err)
	}
	if err := runtime.Request(ActionStop); !errors.Is(err, ErrActionPending) {
		t.Fatalf("action accepted during restart: %v", err)
	}
	runtime.MarkReady()
	status, err = runtime.Status(context.Background())
	if err != nil || status.Pending != "" {
		t.Fatalf("ready runtime retained action: status=%#v err=%v", status, err)
	}
}

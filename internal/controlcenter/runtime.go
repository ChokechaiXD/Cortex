package controlcenter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

type Action string

const (
	ActionRestart Action = "restart"
	ActionStop    Action = "stop"
)

var (
	ErrInvalidAction = errors.New("invalid runtime action")
	ErrActionPending = errors.New("runtime action already pending")
)

type Status struct {
	Running bool
	Version string
	Listen  string
	Port    int
	PID     int
	DataDir string
	Uptime  time.Duration
	Pending Action
}

type Runtime struct {
	version   string
	listen    string
	port      int
	dataDir   string
	startedAt time.Time
	actions   chan Action
	mu        sync.Mutex
	pending   Action
}

func NewRuntime(version, listen, dataDir string) *Runtime {
	_, portText, _ := net.SplitHostPort(listen)
	port, _ := strconv.Atoi(portText)
	return &Runtime{
		version: version, listen: listen, port: port, dataDir: dataDir,
		startedAt: time.Now(), actions: make(chan Action, 1),
	}
}

func (runtime *Runtime) Status(context.Context) (Status, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return Status{
		Running: true, Version: runtime.version, Listen: runtime.listen, Port: runtime.port,
		PID: os.Getpid(), DataDir: runtime.dataDir, Uptime: time.Since(runtime.startedAt), Pending: runtime.pending,
	}, nil
}

func (runtime *Runtime) Request(action Action) error {
	if action != ActionRestart && action != ActionStop {
		return fmt.Errorf("%w: %q", ErrInvalidAction, action)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.pending != "" {
		return ErrActionPending
	}
	runtime.pending = action
	// ponytail: a short delay lets the browser receive the action page before
	// the listener closes; no client-side JavaScript or second control process.
	time.AfterFunc(150*time.Millisecond, func() { runtime.actions <- action })
	return nil
}

func (runtime *Runtime) Next(ctx context.Context) (Action, error) {
	select {
	case action := <-runtime.actions:
		return action, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// MarkReady clears a restart only after the replacement listener owns the
// configured port. This keeps duplicate clicks from scheduling extra cycles.
func (runtime *Runtime) MarkReady() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.pending == ActionRestart {
		runtime.pending = ""
	}
}

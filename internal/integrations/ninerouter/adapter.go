package ninerouter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cortex.local/cortex/internal/hope"
	"cortex.local/cortex/internal/integrationhub"
)

const (
	processKey = "9router"
	address    = "127.0.0.1:20128"
	baseURL    = "http://127.0.0.1:20128"
)

type Adapter struct {
	store   *hope.Hub
	dataDir string
	client  *http.Client
}

func New(store *hope.Hub, dataDir string) *Adapter {
	return &Adapter{store: store, dataDir: dataDir, client: &http.Client{Timeout: 1200 * time.Millisecond}}
}

func (*Adapter) ID() string { return "9router" }

func (adapter *Adapter) Probe(ctx context.Context, _ string) integrationhub.Status {
	status := integrationhub.Status{ID: adapter.ID(), Name: "9Router", State: integrationhub.StateStopped, URL: baseURL}
	managed, owned, _ := adapter.store.ManagedProcess(ctx, processKey)
	managedAlive := owned && processMatches(managed.PID, managed.StartedAt)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/health", nil)
	response, err := adapter.client.Do(request)
	if err == nil {
		_ = response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			status.State = integrationhub.StateExternal
			status.Detail = "กำลังทำงานจากภายนอก HOPE"
			if managedAlive {
				status.State = integrationhub.StateRunning
				status.Managed = true
				status.Detail = "HOPE เป็นผู้เปิด process นี้"
				status.PID = managed.PID
			}
			return status
		}
		status.State = integrationhub.StateDegraded
		status.Detail = fmt.Sprintf("health check ตอบกลับ HTTP %d", response.StatusCode)
		status.Managed = managedAlive
		if managedAlive {
			status.PID = managed.PID
		}
		return status
	}
	if !portAvailable(address) {
		status.State = integrationhub.StateConflict
		status.Detail = "พอร์ต 20128 ถูกใช้อยู่ แต่ไม่ใช่ 9Router ที่ตอบ health check"
		if managedAlive {
			status.State = integrationhub.StateDegraded
			status.Managed = true
			status.PID = managed.PID
			status.Detail = "9Router ที่ HOPE เปิดยังทำงาน แต่ health check ไม่ตอบ"
		}
		return status
	}
	if _, err := exec.LookPath("9router"); err != nil {
		status.State = integrationhub.StateMissing
		status.Detail = "ไม่พบคำสั่ง 9router ใน PATH"
		return status
	}
	status.Detail = "พร้อมเปิดเมื่อเลือก Work Mode"
	return status
}

func (adapter *Adapter) Execute(ctx context.Context, request integrationhub.ActionRequest) integrationhub.ActionResult {
	result := integrationhub.ActionResult{Integration: adapter.ID(), Action: request.Action, Target: request.Target}
	switch request.Action {
	case "start":
		status := adapter.Probe(ctx, "")
		if status.State == integrationhub.StateRunning || status.State == integrationhub.StateExternal {
			result.Status = status
			result.Message = "9Router ทำงานอยู่แล้ว จึงไม่เปิด process ซ้ำ"
			return result
		}
		if status.State == integrationhub.StateConflict || status.State == integrationhub.StateMissing {
			result.Status = status
			result.Err = errors.New(status.Detail)
			return result
		}
		command, display, err := launch(adapter.dataDir)
		if err != nil {
			result.Err = err
			return result
		}
		managed := hope.ManagedProcess{Key: processKey, PID: command.Process.Pid, Command: display, StartedAt: time.Now().UTC()}
		if err := adapter.store.SaveManagedProcess(ctx, managed); err != nil {
			_ = command.Process.Kill()
			result.Err = err
			return result
		}
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			status = adapter.Probe(ctx, "")
			if status.State == integrationhub.StateRunning || status.State == integrationhub.StateExternal {
				result.Status = status
				result.Message = "เปิด 9Router แล้ว"
				return result
			}
			time.Sleep(250 * time.Millisecond)
		}
		result.Status = adapter.Probe(ctx, "")
		result.Err = fmt.Errorf("9Router did not become ready within 12 seconds")
		return result
	case "stop":
		managed, ok, err := adapter.store.ManagedProcess(ctx, processKey)
		if err != nil {
			result.Err = err
			return result
		}
		if !ok || !processMatches(managed.PID, managed.StartedAt) {
			result.Status = adapter.Probe(ctx, "")
			result.Err = fmt.Errorf("HOPE will not stop a 9Router process it does not own")
			return result
		}
		if err := stopProcessTree(managed.PID); err != nil {
			result.Err = err
			return result
		}
		_ = adapter.store.DeleteManagedProcess(ctx, processKey)
		result.Status = integrationhub.Status{ID: adapter.ID(), Name: "9Router", State: integrationhub.StateStopped, URL: baseURL, Detail: "ปิด process ที่ HOPE เป็นผู้เปิดแล้ว"}
		result.Message = "ปิด 9Router แล้ว"
		return result
	case "open":
		result.Status = adapter.Probe(ctx, "")
		result.OpenURL = baseURL
		result.Message = "พร้อมเปิดหน้า 9Router"
		return result
	default:
		result.Err = fmt.Errorf("unsupported 9Router action %q", request.Action)
		return result
	}
}

func launch(dataDir string) (*exec.Cmd, string, error) {
	path, err := exec.LookPath("9router")
	if err != nil {
		return nil, "", fmt.Errorf("find 9Router: %w", err)
	}
	args := []string{"--host", "127.0.0.1", "--port", "20128", "--no-browser", "--tray", "--skip-update"}
	command := commandForScript(path, args)
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, "", err
	}
	logFile, err := os.OpenFile(filepath.Join(logsDir, "9router.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, "", err
	}
	command.Stdout = logFile
	command.Stderr = logFile
	prepareCommand(command)
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return nil, "", fmt.Errorf("start 9Router: %w", err)
	}
	_ = logFile.Close()
	return command, strings.Join(append([]string{path}, args...), " "), nil
}

func portAvailable(address string) bool {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

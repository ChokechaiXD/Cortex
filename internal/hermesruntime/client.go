package hermesruntime

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var pidPattern = regexp.MustCompile(`(?i)\bpid\D{0,8}(\d+)`)
var gatewayLinePattern = regexp.MustCompile(`^[✓✗]\s+([a-zA-Z0-9_-]+)(?:\s+\(current\))?\s+—\s+(.*)$`)

type GatewayStatus struct {
	Installed bool
	Running   bool
	PID       int
	Detail    string
}

type Client struct {
	command string
}

func New() *Client { return &Client{command: "hermes"} }

func (client *Client) Available() bool {
	_, err := exec.LookPath(client.command)
	return err == nil
}

func (client *Client) Run(ctx context.Context, profile string, args ...string) (string, error) {
	if !client.Available() {
		return "", fmt.Errorf("Hermes CLI is not installed or is not in PATH")
	}
	commandArgs := make([]string, 0, len(args)+2)
	if strings.TrimSpace(profile) != "" && profile != "default" {
		commandArgs = append(commandArgs, "-p", profile)
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, client.command, commandArgs...)
	prepareCommand(command)
	output, err := command.CombinedOutput()
	text := trimOutput(string(output), 6000)
	if err != nil {
		return text, fmt.Errorf("Hermes command failed: %w: %s", err, text)
	}
	return text, nil
}

func (client *Client) Gateway(ctx context.Context, profile string) GatewayStatus {
	if !client.Available() {
		return GatewayStatus{Detail: "ไม่พบ Hermes CLI"}
	}
	statusCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := client.Run(statusCtx, profile, "gateway", "status")
	lower := strings.ToLower(output)
	status := GatewayStatus{Installed: true, Detail: humanStatus(output)}
	if matches := pidPattern.FindStringSubmatch(output); len(matches) == 2 {
		status.PID, _ = strconv.Atoi(matches[1])
	}
	status.Running = status.PID > 0 || strings.Contains(lower, "running") && !strings.Contains(lower, "not running")
	if err != nil && !status.Running {
		status.Detail = "Gateway ยังไม่ทำงาน"
	}
	return status
}

func (client *Client) Gateways(ctx context.Context) (map[string]GatewayStatus, error) {
	listCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := client.Run(listCtx, "", "gateway", "list")
	if err != nil {
		return nil, err
	}
	result := map[string]GatewayStatus{}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		match := gatewayLinePattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		status := GatewayStatus{Installed: true, Detail: match[2]}
		if pid := pidPattern.FindStringSubmatch(match[2]); len(pid) == 2 {
			status.PID, _ = strconv.Atoi(pid[1])
			status.Running = status.PID > 0
		}
		result[match[1]] = status
	}
	return result, nil
}

func (client *Client) GatewayAction(ctx context.Context, profile, action string) (GatewayStatus, error) {
	if action != "start" && action != "stop" && action != "restart" {
		return GatewayStatus{}, fmt.Errorf("unsupported Hermes gateway action %q", action)
	}
	actionCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if _, err := client.Run(actionCtx, profile, "gateway", action); err != nil {
		return client.Gateway(ctx, profile), err
	}
	return client.Gateway(ctx, profile), nil
}

func (client *Client) CreateProfile(ctx context.Context, profile string) error {
	profile = strings.TrimSpace(profile)
	if profile == "" || profile == "default" {
		return nil
	}
	createCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err := client.Run(createCtx, "", "profile", "create", profile)
	return err
}

func trimOutput(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func humanStatus(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return trimOutput(line, 240)
		}
	}
	return "Hermes พร้อมใช้งาน"
}

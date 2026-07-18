package telegram

import (
	"context"
	"net/url"
	"strings"

	"cortex.local/cortex/internal/integrationhub"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (*Adapter) ID() string { return "telegram" }

func (*Adapter) Probe(_ context.Context, target string) integrationhub.Status {
	status := integrationhub.Status{ID: "telegram", Name: "Telegram", State: integrationhub.StateExternal, Detail: "ใช้ Telegram Web เป็นหน้าสนทนาเดิม; HOPE เก็บเฉพาะลิงก์"}
	if validURL(target) {
		status.URL = target
	}
	return status
}

func (adapter *Adapter) Execute(ctx context.Context, request integrationhub.ActionRequest) integrationhub.ActionResult {
	result := integrationhub.ActionResult{Integration: adapter.ID(), Action: request.Action, Target: request.Target}
	if request.Action != "open" || !validURL(request.Target) {
		result.Err = &url.Error{Op: "open", URL: request.Target, Err: errInvalidURL{}}
		return result
	}
	result.Status = adapter.Probe(ctx, request.Target)
	result.OpenURL = request.Target
	result.Message = "พร้อมเปิด Telegram"
	return result
}

func validURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && (parsed.Host == "t.me" || parsed.Host == "web.telegram.org")
}

type errInvalidURL struct{}

func (errInvalidURL) Error() string { return "Telegram URL is invalid" }

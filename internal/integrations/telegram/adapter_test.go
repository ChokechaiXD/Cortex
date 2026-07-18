package telegram

import (
	"context"
	"testing"

	"cortex.local/cortex/internal/integrationhub"
)

func TestTelegramIsAnExternalLinkOnly(t *testing.T) {
	t.Parallel()
	adapter := New()
	status := adapter.Probe(context.Background(), "https://t.me/example")
	if status.State != integrationhub.StateExternal || status.Managed || status.URL != "https://t.me/example" {
		t.Fatalf("status=%#v", status)
	}
	invalid := adapter.Execute(context.Background(), integrationhub.ActionRequest{Action: "open", Target: "http://example.com"})
	if invalid.Err == nil {
		t.Fatal("non-Telegram URL was accepted")
	}
}

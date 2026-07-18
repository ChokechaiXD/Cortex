package automations

import "testing"

func TestParseJobsUsesHermesListFormat(t *testing.T) {
	t.Parallel()
	jobs := parseJobs(`Cron Jobs:
4b4997c76412 [active]
  Name: infra-heartbeat
  Schedule: every 10 minutes
  Next run: 2026-07-18 20:10
  Last run: 2026-07-18 20:00
  Deliver: telegram

abcdef123456 [paused]
  Name: nightly-review
  Schedule: daily at 02:00
`)
	if len(jobs) != 2 {
		t.Fatalf("jobs=%#v", jobs)
	}
	if jobs[0].ID != "4b4997c76412" || jobs[0].Status != "active" || jobs[0].Name != "infra-heartbeat" || jobs[0].Deliver != "telegram" {
		t.Fatalf("first job=%#v", jobs[0])
	}
	if jobs[1].Status != "paused" || jobs[1].Schedule != "daily at 02:00" {
		t.Fatalf("second job=%#v", jobs[1])
	}
}

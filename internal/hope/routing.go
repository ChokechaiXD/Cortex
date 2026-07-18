package hope

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func (hub *Hub) Skill(ctx context.Context, id string) (Skill, error) {
	var item Skill
	var keywords, updated string
	var enabled int
	err := hub.db.QueryRowContext(ctx, `SELECT id,name,description,path,source,source_url,keywords_json,role,project,enabled,use_count,success_count,failure_count,updated_at FROM skills WHERE id=?`, strings.TrimSpace(id)).
		Scan(&item.ID, &item.Name, &item.Description, &item.Path, &item.Source, &item.SourceURL, &keywords, &item.Role, &item.Project, &enabled, &item.UseCount, &item.SuccessCount, &item.FailureCount, &updated)
	if err != nil {
		return Skill{}, fmt.Errorf("find skill: %w", err)
	}
	item.Keywords = decodeStrings(keywords)
	item.Enabled = enabled == 1
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item, nil
}

func (hub *Hub) RouteSkills(ctx context.Context, request RouteRequest) ([]SkillMatch, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Limit < 1 || request.Limit > 8 {
		request.Limit = 3
	}
	candidates, err := hub.Skills(ctx)
	if err != nil {
		return nil, err
	}
	terms := routeTerms(request.Query)
	matches := make([]SkillMatch, 0, len(candidates))
	for _, skill := range candidates {
		if !skill.Enabled {
			continue
		}
		haystack := strings.ToLower(strings.Join(append([]string{skill.ID, skill.Name, skill.Description}, skill.Keywords...), " "))
		textHits := 0
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				textHits++
			}
		}
		score := float64(textHits) * 1.8
		reasons := make([]string, 0, 4)
		if textHits > 0 {
			reasons = append(reasons, fmt.Sprintf("ตรงกับคำขอ %d จุด", textHits))
		}
		if request.AgentID != "" && strings.EqualFold(skill.Role, request.AgentID) {
			score += 2.5
			reasons = append(reasons, "ตรงกับเอเจนต์")
		}
		if request.ProjectID != "" && strings.EqualFold(skill.Project, request.ProjectID) {
			score += 3
			reasons = append(reasons, "ตรงกับโปรเจกต์")
		}
		if skill.UseCount > 0 {
			reliability := float64(skill.SuccessCount+1) / float64(skill.SuccessCount+skill.FailureCount+2)
			score += math.Min(1.5, reliability*math.Log2(float64(skill.UseCount)+1))
			reasons = append(reasons, "มีประวัติการใช้งาน")
		}
		if score <= 0 {
			continue
		}
		matches = append(matches, SkillMatch{Skill: skill, Score: score, Reason: strings.Join(reasons, " · ")})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Skill.Name < matches[j].Skill.Name
		}
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > request.Limit {
		matches = matches[:request.Limit]
	}
	return matches, nil
}

func (hub *Hub) RecordSkillOutcome(ctx context.Context, id, outcome string) error {
	success, failure := 0, 0
	switch outcome {
	case "success":
		success = 1
	case "failure":
		failure = 1
	case "used":
	default:
		return fmt.Errorf("unknown skill outcome")
	}
	_, err := hub.db.ExecContext(ctx, `UPDATE skills SET use_count=use_count+1,success_count=success_count+?,failure_count=failure_count+?,updated_at=? WHERE id=?`, success, failure, nowText(), id)
	return err
}

func (hub *Hub) SaveSkillRoute(ctx context.Context, pack ContextPack) (string, error) {
	pack.AgentID = strings.TrimSpace(pack.AgentID)
	pack.IdempotencyKey = strings.TrimSpace(pack.IdempotencyKey)
	pack.Query = strings.TrimSpace(pack.Query)
	pack.Router = strings.TrimSpace(pack.Router)
	if pack.Router == "" {
		pack.Router = "deterministic"
	}
	if pack.AgentID == "" || pack.IdempotencyKey == "" || pack.Query == "" {
		return "", fmt.Errorf("context pack requires agent, idempotency key and query")
	}
	requestKey := scopedKey(pack.AgentID, pack.IdempotencyKey)
	pack.ID = "pack_" + requestKey[:24]
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO context_packs(id,request_key,agent_id,session_id,query,project_id,router,input_tokens,output_tokens,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		pack.ID, requestKey, pack.AgentID, strings.TrimSpace(pack.SessionID), pack.Query, strings.TrimSpace(pack.ProjectID), pack.Router, pack.InputTokens, pack.OutputTokens, nowText())
	if err != nil {
		return "", fmt.Errorf("save context pack: %w", err)
	}
	inserted, _ := result.RowsAffected()
	if inserted == 0 {
		var query, projectID string
		if err := tx.QueryRowContext(ctx, `SELECT query,project_id FROM context_packs WHERE request_key=?`, requestKey).Scan(&query, &projectID); err != nil {
			return "", err
		}
		if query != pack.Query || projectID != strings.TrimSpace(pack.ProjectID) {
			return "", fmt.Errorf("idempotency key was used for another context pack")
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return pack.ID, nil
	}
	for position, match := range pack.Skills {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO context_pack_skills(pack_id,skill_id,position,score,reason) VALUES(?,?,?,?,?)`,
			pack.ID, match.Skill.ID, position+1, match.Score, match.Reason); err != nil {
			return "", fmt.Errorf("save context pack route: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return pack.ID, nil
}

func (hub *Hub) ApplySkillFeedback(ctx context.Context, input SkillFeedback) error {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.PackID = strings.TrimSpace(input.PackID)
	input.SkillID = strings.TrimSpace(input.SkillID)
	if input.AgentID == "" || input.IdempotencyKey == "" || input.PackID == "" || input.SkillID == "" {
		return fmt.Errorf("skill feedback requires agent, idempotency key, context pack and skill")
	}
	if input.Outcome != "used" && input.Outcome != "success" && input.Outcome != "failure" {
		return fmt.Errorf("skill feedback outcome must be used, success or failure")
	}
	tx, err := hub.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT agent_id FROM context_packs WHERE id=?`, input.PackID).Scan(&owner); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("context pack not found")
		}
		return err
	}
	if owner != input.AgentID {
		return fmt.Errorf("context pack belongs to another agent")
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM context_pack_skills WHERE pack_id=? AND skill_id=?`, input.PackID, input.SkillID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("skill was not recommended in this context pack")
		}
		return err
	}
	requestKey := scopedKey(input.AgentID, input.IdempotencyKey)
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO skill_feedback_events(request_key,pack_id,skill_id,agent_id,outcome,created_at) VALUES(?,?,?,?,?,?)`,
		requestKey, input.PackID, input.SkillID, input.AgentID, input.Outcome, nowText())
	if err != nil {
		return fmt.Errorf("record skill feedback: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		var packID, skillID, outcome string
		if err := tx.QueryRowContext(ctx, `SELECT pack_id,skill_id,outcome FROM skill_feedback_events WHERE request_key=?`, requestKey).
			Scan(&packID, &skillID, &outcome); err != nil {
			return err
		}
		if packID != input.PackID || skillID != input.SkillID || outcome != input.Outcome {
			return fmt.Errorf("idempotency key was used for different skill feedback")
		}
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE context_pack_skills SET outcome=?,feedback_at=? WHERE pack_id=? AND skill_id=?`,
		input.Outcome, nowText(), input.PackID, input.SkillID); err != nil {
		return err
	}
	success, failure := 0, 0
	if input.Outcome == "success" {
		success = 1
	}
	if input.Outcome == "failure" {
		failure = 1
	}
	if _, err := tx.ExecContext(ctx, `UPDATE skills SET use_count=use_count+1,success_count=success_count+?,failure_count=failure_count+?,updated_at=? WHERE id=?`,
		success, failure, nowText(), input.SkillID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO action_events(id,target,action,status,message,created_at) VALUES(?,?,?,?,?,?)`,
		"skillfb_"+requestKey[:24], "skill:"+input.SkillID, "feedback", "ok", input.Outcome+" via "+input.PackID, nowText()); err != nil {
		return err
	}
	return tx.Commit()
}

func scopedKey(actorID, requestKey string) string {
	sum := sha256.Sum256([]byte(actorID + "\x00" + requestKey))
	return hex.EncodeToString(sum[:])
}

func routeTerms(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	seen := map[string]bool{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, ".,!?;:()[]{}\"'")
		if len([]rune(part)) < 2 || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
	}
	return result
}

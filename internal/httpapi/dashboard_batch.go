package httpapi

import (
	"net/http"
	"strings"

	"cortex.local/cortex/internal/cortex"
)

func (server *Server) dashboardReviewBatch(writer http.ResponseWriter, request *http.Request) {
	_, session, ok := server.sessions.fromRequest(request)
	if !ok {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	if !server.hub.CanGovern(session.AgentID) {
		http.Error(writer, "batch review is not permitted", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<10)
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid batch review", http.StatusBadRequest)
		return
	}
	if !validCSRF(session.CSRFToken, request.FormValue("csrf")) {
		http.Error(writer, "invalid csrf token", http.StatusForbidden)
		return
	}
	if request.FormValue("confirm") != "yes" {
		http.Error(writer, "batch review confirmation is required", http.StatusBadRequest)
		return
	}
	requestID, err := dashboardRequestID()
	if err != nil {
		http.Error(writer, "create batch review id", http.StatusInternalServerError)
		return
	}
	_, err = server.hub.ReviewBatch(request.Context(), cortex.ReviewBatchCommand{
		IdempotencyKey: requestID, MemoryIDs: request.Form["memory_id"], ActorID: session.AgentID,
		Decision: cortex.ReviewDecision(request.FormValue("decision")),
		Reason:   strings.TrimSpace(request.FormValue("reason")),
	})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	http.Redirect(writer, request, "/?lifecycle=candidate&batch=reviewed", http.StatusSeeOther)
}

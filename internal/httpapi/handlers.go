package httpapi

import (
	"net/http"

	"cortex.local/cortex/internal/cortex"
)

func (server *Server) remember(writer http.ResponseWriter, request *http.Request) {
	key, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command cortex.RememberCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeAPIError(writer, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	command.IdempotencyKey = key
	command.AgentID = identityFromRequest(request)
	memory, err := server.hub.Remember(request.Context(), command)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, memory)
}

func (server *Server) recall(writer http.ResponseWriter, request *http.Request) {
	key, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var query cortex.RecallQuery
	if err := decodeJSON(writer, request, &query); err != nil {
		writeAPIError(writer, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	query.IdempotencyKey = key
	query.AgentID = identityFromRequest(request)
	result, err := server.hub.Recall(request.Context(), query)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) feedback(writer http.ResponseWriter, request *http.Request) {
	key, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command cortex.FeedbackCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeAPIError(writer, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	command.IdempotencyKey = key
	command.MemoryID = request.PathValue("memoryID")
	command.AgentID = identityFromRequest(request)
	memory, err := server.hub.Feedback(request.Context(), command)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, memory)
}

func (server *Server) review(writer http.ResponseWriter, request *http.Request) {
	key, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command cortex.ReviewCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeAPIError(writer, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	command.IdempotencyKey = key
	command.MemoryID = request.PathValue("memoryID")
	command.ActorID = identityFromRequest(request)
	memory, err := server.hub.Review(request.Context(), command)
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, memory)
}

func (server *Server) history(writer http.ResponseWriter, request *http.Request) {
	events, err := server.hub.History(request.Context(), cortex.HistoryQuery{
		MemoryID: request.PathValue("memoryID"),
		AgentID:  identityFromRequest(request),
	})
	if err != nil {
		writeDomainError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"events": events})
}

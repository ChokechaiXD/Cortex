package httpapi

import (
	"context"
	"net/http"
	"strings"
)

type Authenticator interface {
	Authenticate(token string) (agentID string, ok bool)
}

type StaticAuthenticator map[string]string

func (auth StaticAuthenticator) Authenticate(token string) (string, bool) {
	agentID, ok := auth[token]
	return agentID, ok
}

type identityContextKey struct{}

func withIdentity(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, identityContextKey{}, agentID)
}

func identityFromRequest(request *http.Request) string {
	agentID, _ := request.Context().Value(identityContextKey{}).(string)
	return agentID
}

func bearerToken(request *http.Request) string {
	header := request.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func authenticationToken(request *http.Request) string {
	if token := bearerToken(request); token != "" {
		return token
	}
	cookie, err := request.Cookie(dashboardCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

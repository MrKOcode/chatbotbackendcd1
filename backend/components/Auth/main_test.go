package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/golang-jwt/jwt/v4"
)

func stubTokenParser(t *testing.T, fn func(string) (*jwt.Token, error)) {
	t.Helper()
	previous := parseToken
	parseToken = fn
	t.Cleanup(func() { parseToken = previous })
}

func validToken(claims jwt.MapClaims) *jwt.Token {
	return &jwt.Token{Valid: true, Claims: claims}
}

func TestToJSONStringArray(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "empty", want: "[]"},
		{name: "values", in: []string{"students", `quote"inside`}, want: `["students","quote\"inside"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toJSONStringArray(tt.in); got != tt.want {
				t.Fatalf("toJSONStringArray() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestHandlerPreflight(t *testing.T) {
	resp, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "OPTIONS"})
	if err != nil || resp.StatusCode != http.StatusOK || resp.Body != "ok" {
		t.Fatalf("unexpected preflight response: %#v, err=%v", resp, err)
	}
	assertCORSHeaders(t, resp)
}

func TestHandlerRequiresAuthorization(t *testing.T) {
	resp, err := handler(context.Background(), events.APIGatewayProxyRequest{})
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected response: %#v, err=%v", resp, err)
	}
	if resp.Body != "Missing Authorization header" {
		t.Fatalf("unexpected body: %q", resp.Body)
	}
}

func TestHandlerRejectsInvalidToken(t *testing.T) {
	stubTokenParser(t, func(got string) (*jwt.Token, error) {
		if got != "bad-token" {
			t.Fatalf("parser received %q", got)
		}
		return nil, errors.New("invalid signature")
	})
	resp, _ := handler(context.Background(), events.APIGatewayProxyRequest{
		Headers: map[string]string{"Authorization": "Bearer bad-token"},
	})
	if resp.StatusCode != http.StatusUnauthorized || resp.Body != "Invalid token" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestHandlerRejectsTokenMarkedInvalid(t *testing.T) {
	stubTokenParser(t, func(string) (*jwt.Token, error) {
		return &jwt.Token{Valid: false}, nil
	})
	resp, _ := handler(context.Background(), events.APIGatewayProxyRequest{
		Headers: map[string]string{"Authorization": "Bearer token"},
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got status %d", resp.StatusCode)
	}
}

func TestHandlerReturnsStudentClaims(t *testing.T) {
	stubTokenParser(t, func(string) (*jwt.Token, error) {
		return validToken(jwt.MapClaims{
			"sub":            "student-1",
			"email":          "student@example.com",
			"cognito:groups": []interface{}{"students", 7},
		}), nil
	})
	resp, err := handler(context.Background(), events.APIGatewayProxyRequest{
		Headers: map[string]string{"Authorization": "Bearer token"},
	})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response: %#v, err=%v", resp, err)
	}
	var body struct {
		UserID string   `json:"userId"`
		Email  string   `json:"email"`
		Role   string   `json:"role"`
		Groups []string `json:"groups"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.UserID != "student-1" || body.Email != "student@example.com" || body.Role != "student" {
		t.Fatalf("unexpected body: %+v", body)
	}
	if len(body.Groups) != 1 || body.Groups[0] != "students" {
		t.Fatalf("unexpected groups: %v", body.Groups)
	}
	assertCORSHeaders(t, resp)
}

func TestHandlerReturnsAdminRole(t *testing.T) {
	stubTokenParser(t, func(string) (*jwt.Token, error) {
		return validToken(jwt.MapClaims{
			"sub":            "admin-1",
			"email":          "admin@example.com",
			"cognito:groups": []interface{}{"students", "admins"},
		}), nil
	})
	resp, _ := handler(context.Background(), events.APIGatewayProxyRequest{
		Headers: map[string]string{"Authorization": "raw-token"},
	})
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["role"] != "admin" {
		t.Fatalf("expected admin role, got %v", body["role"])
	}
}

func TestInitJWKSRequiresURL(t *testing.T) {
	t.Setenv("COGNITO_JWKS_URL", "")
	if err := initJWKS(); err == nil {
		t.Fatal("expected missing URL error")
	}
}

func TestResponseHelpers(t *testing.T) {
	for _, resp := range []events.APIGatewayProxyResponse{
		response(http.StatusCreated, `{"ok":true}`),
		corsResponse(http.StatusBadRequest, "bad request"),
	} {
		assertCORSHeaders(t, resp)
	}
}

func assertCORSHeaders(t *testing.T, resp events.APIGatewayProxyResponse) {
	t.Helper()
	if resp.Headers["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("missing CORS origin in %#v", resp.Headers)
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Fatalf("unexpected content type in %#v", resp.Headers)
	}
}

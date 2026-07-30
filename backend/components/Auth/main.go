package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/golang-jwt/jwt/v4"
)

var jwks *keyfunc.JWKS

var parseToken = func(tokenString string) (*jwt.Token, error) {
	if jwks == nil {
		return nil, fmt.Errorf("JWKS is not initialized")
	}
	return jwt.Parse(tokenString, jwks.Keyfunc)
}

func initJWKS() error {
	jwksURL := os.Getenv("COGNITO_JWKS_URL")
	if jwksURL == "" {
		return fmt.Errorf("COGNITO_JWKS_URL not set in environment")
	}

	var err error
	jwks, err = keyfunc.Get(jwksURL, keyfunc.Options{
		RefreshInterval: time.Hour,
	})
	if err != nil {
		return fmt.Errorf("failed to get JWKS: %w", err)
	}
	return nil
}

func toJSONStringArray(in []string) string {
	if len(in) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range in {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf("%q", s))
	}
	b.WriteString("]")
	return b.String()
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// ✅ Handle CORS preflight (OPTIONS)
	if req.HTTPMethod == "OPTIONS" {
		return corsResponse(200, "ok"), nil
	}

	authHeader := req.Headers["Authorization"]
	if authHeader == "" {
		return corsResponse(http.StatusUnauthorized, "Missing Authorization header"), nil
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := parseToken(tokenStr)
	if err != nil || !token.Valid {
		return corsResponse(http.StatusUnauthorized, "Invalid token"), nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return corsResponse(http.StatusInternalServerError, "Invalid claims"), nil
	}

	sub := claims["sub"]
	email := claims["email"]
	// cognito:groups is usually []interface{} when parsed from JWT
	var groups []string
	if raw, ok := claims["cognito:groups"].([]interface{}); ok {
		for _, g := range raw {
			if s, ok := g.(string); ok {
				groups = append(groups, s)
			}
		}
	}

	role := "student"
	for _, g := range groups {
		if g == "admins" {
			role = "admin"
			break
		}
	}

	body := fmt.Sprintf(`{"userId":%q,"email":%q,"role":%q,"groups":%s}`,
		sub, email, role, toJSONStringArray(groups),
	)

	return response(http.StatusOK, body), nil
}

// ✅ Add CORS headers
func corsResponse(status int, msg string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body:       msg,
		Headers: map[string]string{
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST,PUT,DELETE",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
			"Content-Type":                 "application/json",
		},
	}
}

func response(status int, msg string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body:       msg,
		Headers: map[string]string{
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST,PUT,DELETE",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
			"Content-Type":                 "application/json",
		},
	}
}

func main() {
	if err := initJWKS(); err != nil {
		log.Fatal(err)
	}
	lambda.Start(handler)
}

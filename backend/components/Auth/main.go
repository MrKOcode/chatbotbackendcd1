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

func init() {
	jwksURL := os.Getenv("COGNITO_JWKS_URL")
	if jwksURL == "" {
		log.Fatal("COGNITO_JWKS_URL not set in environment")
	}

	var err error
	jwks, err = keyfunc.Get(jwksURL, keyfunc.Options{
		RefreshInterval: time.Hour,
	})
	if err != nil {
		log.Fatalf("Failed to get JWKS: %v", err)
	}
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

	token, err := jwt.Parse(tokenStr, jwks.Keyfunc)
	if err != nil || !token.Valid {
		return corsResponse(http.StatusUnauthorized, "Invalid token"), nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return corsResponse(http.StatusInternalServerError, "Invalid claims"), nil
	}

	sub := claims["sub"]
	email := claims["email"]
	body := fmt.Sprintf("Authenticated: sub=%v, email=%v", sub, email)
	// ✅ Normal success uses response() (includes same CORS headers)
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
	lambda.Start(handler)
}

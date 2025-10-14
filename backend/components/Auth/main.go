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
	authHeader := req.Headers["Authorization"]
	if authHeader == "" {
		return response(http.StatusUnauthorized, "Missing Authorization header"), nil
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := jwt.Parse(tokenStr, jwks.Keyfunc)
	if err != nil || !token.Valid {
		return response(http.StatusUnauthorized, "Invalid token"), nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return response(http.StatusInternalServerError, "Invalid claims"), nil
	}

	// You can access specific fields like:
	sub := claims["sub"]
	email := claims["email"]
	// Return these as confirmation
	msg := fmt.Sprintf("Authenticated: sub=%v, email=%v", sub, email)

	return response(http.StatusOK, msg), nil
}

func response(status int, msg string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body:       msg,
	}
}

func main() {
	lambda.Start(handler)
}

// ✅ main.go — Fully DynamoDB-Integrated (AIChat Lambda)

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
	"github.com/oklog/ulid/v2"

	"github.com/MrKOcode/AiChatBot3.0backenddeploy/backend/components/AIChat/services"
)

var getChatResponse = services.GetChatGPTResponse

func main() {
	_ = godotenv.Load(".env")
	if err := services.InitDAL(); err != nil {
		panic("DAL init failed: " + err.Error())
	}
	lambda.Start(handler)
}

func handler(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.HTTPMethod == "OPTIONS" {
		return corsResponse(200, "ok"), nil
	}

	// Normalize stage prefix
	path := strings.TrimPrefix(req.Path, "/"+apiStageName())
	path = strings.TrimSuffix(path, "/")

	switch req.HTTPMethod {

	case "GET":
		if path == "/api/AIchat/conversations" {
			return lambdaFetchConversations(req)
		}
		if path == "/api/AIchat/history" {
			return lambdaFetchChatHistory(req)
		}
		if strings.Contains(path, "/messages") {
			return lambdaFetchMessages(req) // <-- you must implement this
		}

	case "POST":
		if path == "/api/AIchat/conversations" {
			return lambdaCreateConversation(req)
		}
		if strings.Contains(path, "/messages") {
			return lambdaSendMessage(req)
		}

	case "DELETE":
		if strings.Contains(path, "/conversations/") {
			return lambdaDeleteConversation(req)
		}
	}

	return errorResponse(404, "Route not found"), nil
}

// ========== Handlers ==========

func lambdaCreateConversation(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	auth, err := getAuthInfo(req)
	if err != nil {
		return errorResponse(401, err.Error()), nil
	}

	// normal users create for themselves; admins also create for themselves unless targetUserId is explicitly provided
	userID, err := resolveEffectiveUserID(req, auth)
	if err != nil {
		return errorResponse(403, err.Error()), nil
	}

	id, err := services.Store.CreateConversation(context.Background(), userID, "New Academic Chat")
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}

	greeting := "This is your personal AiChatBot, what can I help you study today?"
	err = services.Store.PutMessage(context.Background(), services.ChatMessage{
		ID:             generateULID(),
		ConversationID: id,
		UserID:         userID,
		Role:           "chatbot",
		Content:        greeting,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		return errorResponse(500, "Failed to save greeting message"), nil
	}

	return jsonResponse(200, map[string]interface{}{
		"conversationId": id,
		"conversation":   map[string]string{"title": "New Academic Chat"},
	}), nil
}

func lambdaFetchConversations(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	auth, err := getAuthInfo(req)
	if err != nil {
		return errorResponse(401, err.Error()), nil
	}

	userID, err := resolveEffectiveUserID(req, auth)
	if err != nil {
		return errorResponse(403, err.Error()), nil
	}

	page, err := services.Store.ListConversations(context.Background(), userID, 20, "")
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}
	return jsonResponse(200, map[string]interface{}{
		"content": map[string]interface{}{
			"data": page.Items,
		},
	}), nil
}

func lambdaSendMessage(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	auth, err := getAuthInfo(req)
	if err != nil {
		return errorResponse(401, err.Error()), nil
	}

	userID, err := resolveEffectiveUserID(req, auth)
	if err != nil {
		return errorResponse(403, err.Error()), nil
	}

	var body struct {
		Message services.ChatMessage `json:"message"`
	}
	_ = json.Unmarshal([]byte(req.Body), &body)

	if body.Message.ConversationID == "" {
		return errorResponse(400, "Missing conversationId"), nil
	}
	if strings.TrimSpace(body.Message.Content) == "" {
		return errorResponse(400, "Missing message content"), nil
	}
	if _, err := services.Store.GetConversation(context.Background(), userID, body.Message.ConversationID); err != nil {
		return errorResponse(403, "Forbidden"), nil
	}

	now := time.Now().UTC()
	userMsg := services.ChatMessage{
		ID:             generateULID(),
		ConversationID: body.Message.ConversationID,
		UserID:         userID,
		Role:           "user",
		Content:        body.Message.Content,
		CreatedAt:      now,
	}
	if err := services.Store.PutMessage(context.Background(), userMsg); err != nil {
		return errorResponse(500, err.Error()), nil
	}

	// Simulated reply for now
	botReply, err := getChatResponse(body.Message.Content)
	if err != nil {
		return errorResponse(500, "Failed to get AI response: "+err.Error()), nil
	}
	botMsg := services.ChatMessage{
		ID:             generateULID(),
		ConversationID: body.Message.ConversationID,
		UserID:         userID,
		Role:           "chatbot",
		Content:        botReply,
		CreatedAt:      now.Add(time.Millisecond),
	}
	if err := services.Store.PutMessage(context.Background(), botMsg); err != nil {
		return errorResponse(500, err.Error()), nil
	}

	return jsonResponse(200, map[string]interface{}{
		"message":  userMsg,
		"response": botMsg,
	}), nil
}

func lambdaDeleteConversation(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	auth, err := getAuthInfo(req)
	if err != nil {
		return errorResponse(401, err.Error()), nil
	}

	normalized := strings.TrimPrefix(req.Path, "/"+apiStageName())
	conversationID := strings.TrimPrefix(normalized, "/api/AIchat/conversations/")
	if conversationID == "" {
		return errorResponse(400, "Missing conversationId"), nil
	}

	userID, err := resolveEffectiveUserID(req, auth)
	if err != nil {
		return errorResponse(403, err.Error()), nil
	}

	// Ownership check
	if _, err := services.Store.GetConversation(context.Background(), userID, conversationID); err != nil {
		return errorResponse(403, "Forbidden"), nil
	}

	ctx := context.Background()
	if err := services.Store.DeleteConversationCascade(ctx, userID, conversationID); err != nil {
		return errorResponse(500, err.Error()), nil
	}
	return jsonResponse(200, map[string]string{"conversationId": conversationID}), nil
}

func lambdaFetchChatHistory(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	auth, err := getAuthInfo(req)
	if err != nil {
		return errorResponse(401, err.Error()), nil
	}

	// Normal user -> auth.Sub; Admin -> can override with ?targetUserId=...
	userID, err := resolveEffectiveUserID(req, auth)
	if err != nil {
		return errorResponse(403, err.Error()), nil
	}

	page, err := services.Store.ListUserMessagesSince(
		context.Background(),
		userID,
		time.Time{},
		50,
		"",
	)
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}

	var history []map[string]string
	msgs := page.Items

	for i := 0; i < len(msgs)-1; i++ {
		if msgs[i].Role == "user" && msgs[i+1].Role == "chatbot" && msgs[i].ConversationID == msgs[i+1].ConversationID {
			history = append(history, map[string]string{
				"userMessage": msgs[i].Content,
				"response":    msgs[i+1].Content,
				"timestamp":   msgs[i+1].CreatedAt.Format(time.RFC3339),
			})
			i++
		}
		if len(history) == 5 {
			break
		}
	}

	return jsonResponse(200, map[string]interface{}{"history": history}), nil
}

func lambdaFetchMessages(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	auth, err := getAuthInfo(req)
	if err != nil {
		return errorResponse(401, err.Error()), nil
	}

	// Extract conversationId from path
	normalized := strings.TrimPrefix(req.Path, "/"+apiStageName())
	parts := strings.Split(normalized, "/")
	if len(parts) < 6 {
		return errorResponse(400, "Invalid messages path"), nil
	}
	// /api/AIchat/conversations/{id}/messages
	conversationID := parts[4]
	if conversationID == "" {
		return errorResponse(400, "Missing conversationId"), nil
	}

	// Determine which user we are operating on (admin can pass targetUserId)
	userID, err := resolveEffectiveUserID(req, auth)
	if err != nil {
		return errorResponse(403, err.Error()), nil
	}

	// SECURITY CHECK: confirm this conversation belongs to userID.
	// Uses your existing DAL method (returns error if not found / not owned).
	if _, err := services.Store.GetConversation(context.Background(), userID, conversationID); err != nil {
		// treat as forbidden to avoid leaking existence
		return errorResponse(403, "Forbidden"), nil
	}

	page, err := services.Store.ListMessages(
		context.Background(),
		conversationID,
		100,
		"",
		false,
	)
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}

	return jsonResponse(200, map[string]interface{}{
		"content": map[string]interface{}{
			"conversationId": conversationID,
			"content":        page.Items,
			"pagination": map[string]interface{}{
				"hasMore":   page.NextToken != "",
				"nextToken": page.NextToken,
			},
		},
	}), nil
}

type AuthInfo struct {
	Sub     string
	IsAdmin bool
	Groups  []string
}

// Works with REST API + Cognito Authorizer where claims are in req.RequestContext.Authorizer["claims"]
func getAuthInfo(req events.APIGatewayProxyRequest) (AuthInfo, error) {
	a := AuthInfo{}

	rawClaims, ok := req.RequestContext.Authorizer["claims"]
	if !ok || rawClaims == nil {
		return a, errors.New("missing authorizer claims (check API Gateway authorizer config)")
	}

	claims, ok := rawClaims.(map[string]interface{})
	if !ok {
		return a, errors.New("invalid claims type")
	}

	// sub
	if v, ok := claims["sub"]; ok {
		if s, ok := v.(string); ok && s != "" {
			a.Sub = s
		}
	}
	if a.Sub == "" {
		return a, errors.New("missing sub claim")
	}

	// groups: often a string like "admins,students" in REST API authorizer claims
	if v, ok := claims["cognito:groups"]; ok && v != nil {
		switch t := v.(type) {
		case string:
			for _, g := range strings.Split(t, ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					a.Groups = append(a.Groups, g)
				}
			}
		case []interface{}:
			for _, it := range t {
				if s, ok := it.(string); ok && s != "" {
					a.Groups = append(a.Groups, s)
				}
			}
		}
	}

	for _, g := range a.Groups {
		if g == "admins" {
			a.IsAdmin = true
			break
		}
	}

	return a, nil
}

// Decide which userId to operate on.
// - Normal user: always their own sub
// - Admin: can pass targetUserId (query param) to view other users
func resolveEffectiveUserID(req events.APIGatewayProxyRequest, auth AuthInfo) (string, error) {
	target := strings.TrimSpace(req.QueryStringParameters["targetUserId"])
	if target == "" {
		return auth.Sub, nil
	}
	if !auth.IsAdmin {
		return "", errors.New("forbidden: targetUserId is admin-only")
	}
	return target, nil
}

// ========== Helpers ==========

func jsonResponse(status int, data interface{}) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(data)
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body:       string(body),
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  allowedOrigin(),
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST,PUT,DELETE",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
		},
	}
}

func allowedOrigin() string {
	if origin := os.Getenv("ALLOWED_ORIGIN"); origin != "" {
		return origin
	}
	return "http://mychatbot-frontend3-0.s3-website-us-west-2.amazonaws.com"
}

func apiStageName() string {
	if stageName := os.Getenv("API_STAGE_NAME"); stageName != "" {
		return stageName
	}
	return "Prod"
}

func errorResponse(status int, msg string) events.APIGatewayProxyResponse {
	return jsonResponse(status, map[string]string{"error": msg})
}

func generateULID() string {
	return ulid.Make().String() // you can implement this helper in dynamo_dal.go if needed
}

// handle CORS preflight
func corsResponse(status int, msg string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body:       msg,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  allowedOrigin(),
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST,PUT,DELETE",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
		},
	}
}

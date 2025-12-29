// ✅ main.go — Fully DynamoDB-Integrated (AIChat Lambda)

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
	"github.com/oklog/ulid/v2"

	"github.com/MrKOcode/AiChatBot3.0backenddeploy/backend/components/AIChat/services"
)

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
	path := strings.TrimPrefix(req.Path, "/Prod")

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
	userId := req.QueryStringParameters["userId"]
	if userId == "" {
		return errorResponse(400, "Missing userId"), nil
	}

	page, err := services.Store.ListConversations(context.Background(), userId, 20, "")
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}
	return jsonResponse(200, map[string]interface{}{"content": map[string]interface{}{"data": page.Items}}), nil
}

func lambdaSendMessage(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var body struct {
		UserID  string               `json:"userId"`
		Message services.ChatMessage `json:"message"`
	}
	_ = json.Unmarshal([]byte(req.Body), &body)

	now := time.Now().UTC()
	userMsg := services.ChatMessage{
		ID:             generateULID(),
		ConversationID: body.Message.ConversationID,
		UserID:         body.UserID,
		Role:           "user",
		Content:        body.Message.Content,
		CreatedAt:      now,
	}
	_ = services.Store.PutMessage(context.Background(), userMsg)

	// Simulated reply for now
	botReply := fmt.Sprintf("You said: %s", body.Message.Content)
	botMsg := services.ChatMessage{
		ID:             generateULID(),
		ConversationID: body.Message.ConversationID,
		UserID:         body.UserID,
		Role:           "chatbot",
		Content:        botReply,
		CreatedAt:      now.Add(time.Millisecond),
	}
	_ = services.Store.PutMessage(context.Background(), botMsg)

	return jsonResponse(200, map[string]string{"response": botReply}), nil
}

func lambdaDeleteConversation(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	normalized := strings.TrimPrefix(req.Path, "/Prod")
	conversationID := strings.TrimPrefix(normalized, "/api/AIchat/conversations/")
	err := services.Store.DeleteConversationCascade(context.Background(), conversationID)
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}
	return jsonResponse(200, map[string]string{"conversationId": conversationID}), nil
}

func lambdaFetchChatHistory(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	userId := req.QueryStringParameters["userId"]
	if userId == "" {
		return errorResponse(400, "Missing userId"), nil
	}
	page, err := services.Store.ListUserMessagesSince(context.Background(), userId, time.Now().Add(-24*time.Hour), 50, "")
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
			i++ // skip the bot response in next loop
		}
		if len(history) == 5 {
			break
		}
	}

	return jsonResponse(200, map[string]interface{}{"history": history}), nil
}

func lambdaFetchMessages(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract conversationId from path
	normalized := strings.TrimPrefix(req.Path, "/Prod")
	// Example normalized path:
	// /api/AIchat/conversations/123/messages
	parts := strings.Split(normalized, "/")
	if len(parts) < 5 {
		return errorResponse(400, "Invalid messages path"), nil
	}
	conversationID := parts[4] // index 0="",1=api,2=AIchat,3=conversations,4=id

	// Extract userId
	userId := req.QueryStringParameters["userId"]
	if userId == "" {
		return errorResponse(400, "Missing userId"), nil
	}

	// Fetch messages from DynamoDB
	page, err := services.Store.ListMessages(
		context.Background(),
		conversationID,
		100,   // limit
		"",    // nextToken
		false, // newestFirst
	)
	if err != nil {
		return errorResponse(500, err.Error()), nil
	}

	// Format response for frontend
	return jsonResponse(200, map[string]interface{}{
		"userId":         userId,
		"conversationId": conversationID,
		"content": map[string]interface{}{
			"userId":         userId,
			"conversationId": conversationID,
			"content":        page.Items, // array of ChatMessage
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
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST,PUT,DELETE",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
		},
	}
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
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST,PUT,DELETE",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
		},
	}
}

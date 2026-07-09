package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/MrKOcode/AiChatBot3.0backenddeploy/backend/components/AIChat/services"
)

type fakeStore struct {
	listUserID string
	listSince  time.Time

	getConversationErr error
	putMessageCount    int
}

func (f *fakeStore) CreateConversation(ctx context.Context, userID, title string) (string, error) {
	return "conversation-1", nil
}

func (f *fakeStore) ListConversations(ctx context.Context, userID string, limit int32, nextToken string) (services.ListPage[services.Conversation], error) {
	return services.ListPage[services.Conversation]{}, nil
}

func (f *fakeStore) PutMessage(ctx context.Context, m services.ChatMessage) error {
	f.putMessageCount++
	return nil
}

func (f *fakeStore) ListMessages(ctx context.Context, conversationID string, limit int32, nextToken string, newestFirst bool) (services.ListPage[services.ChatMessage], error) {
	return services.ListPage[services.ChatMessage]{}, nil
}

func (f *fakeStore) DeleteConversationCascade(ctx context.Context, userID, conversationID string) error {
	return nil
}

func (f *fakeStore) ListUserMessagesSince(ctx context.Context, userID string, since time.Time, limit int32, nextToken string) (services.ListPage[services.ChatMessage], error) {
	f.listUserID = userID
	f.listSince = since
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	return services.ListPage[services.ChatMessage]{
		Items: []services.ChatMessage{
			{ConversationID: "conversation-1", UserID: userID, Role: "user", Content: "What is photosynthesis?", CreatedAt: now},
			{ConversationID: "conversation-1", UserID: userID, Role: "chatbot", Content: "Photosynthesis converts light into chemical energy.", CreatedAt: now.Add(time.Second)},
		},
	}, nil
}

func (f *fakeStore) GetConversation(ctx context.Context, userID, conversationID string) (services.Conversation, error) {
	if f.getConversationErr != nil {
		return services.Conversation{}, f.getConversationErr
	}
	return services.Conversation{ID: conversationID, UserID: userID}, nil
}

func withFakeStore(t *testing.T, store *fakeStore) {
	t.Helper()
	previous := services.Store
	services.Store = store
	t.Cleanup(func() {
		services.Store = previous
	})
}

func authRequest(sub string, groups interface{}) events.APIGatewayProxyRequest {
	claims := map[string]interface{}{"sub": sub}
	if groups != nil {
		claims["cognito:groups"] = groups
	}
	return events.APIGatewayProxyRequest{
		RequestContext: events.APIGatewayProxyRequestContext{
			Authorizer: map[string]interface{}{
				"claims": claims,
			},
		},
		QueryStringParameters: map[string]string{},
	}
}

func TestFetchChatHistoryUsesAuthenticatedUser(t *testing.T) {
	store := &fakeStore{}
	withFakeStore(t, store)

	req := authRequest("student-1", nil)
	req.QueryStringParameters["userId"] = "student-2"

	resp, err := lambdaFetchChatHistory(req)
	if err != nil {
		t.Fatalf("lambdaFetchChatHistory returned error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d with body %s", resp.StatusCode, resp.Body)
	}
	if store.listUserID != "student-1" {
		t.Fatalf("expected history query for authenticated user student-1, got %q", store.listUserID)
	}
	if !store.listSince.IsZero() {
		t.Fatalf("expected history query without 24-hour cutoff, got since %s", store.listSince)
	}

	var body struct {
		History []map[string]string `json:"history"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if len(body.History) != 1 {
		t.Fatalf("expected one history pair, got %d", len(body.History))
	}
}

func TestFetchChatHistoryRejectsStudentTargetUserID(t *testing.T) {
	store := &fakeStore{}
	withFakeStore(t, store)

	req := authRequest("student-1", nil)
	req.QueryStringParameters["targetUserId"] = "student-2"

	resp, err := lambdaFetchChatHistory(req)
	if err != nil {
		t.Fatalf("lambdaFetchChatHistory returned error: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected status 403, got %d with body %s", resp.StatusCode, resp.Body)
	}
	if store.listUserID != "" {
		t.Fatalf("expected no history query for forbidden request, got user %q", store.listUserID)
	}
}

func TestFetchChatHistoryAllowsAdminTargetUserID(t *testing.T) {
	store := &fakeStore{}
	withFakeStore(t, store)

	req := authRequest("admin-1", "admins")
	req.QueryStringParameters["targetUserId"] = "student-2"

	resp, err := lambdaFetchChatHistory(req)
	if err != nil {
		t.Fatalf("lambdaFetchChatHistory returned error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d with body %s", resp.StatusCode, resp.Body)
	}
	if store.listUserID != "student-2" {
		t.Fatalf("expected admin history query for target student-2, got %q", store.listUserID)
	}
}

func TestSendMessageRequiresOwnedConversationBeforeWrite(t *testing.T) {
	store := &fakeStore{getConversationErr: errors.New("conversation not found")}
	withFakeStore(t, store)

	req := authRequest("student-1", nil)
	req.Body = `{"message":{"conversationId":"conversation-2","content":"hello"}}`

	resp, err := lambdaSendMessage(req)
	if err != nil {
		t.Fatalf("lambdaSendMessage returned error: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected status 403, got %d with body %s", resp.StatusCode, resp.Body)
	}
	if store.putMessageCount != 0 {
		t.Fatalf("expected no messages to be written before ownership check, wrote %d", store.putMessageCount)
	}
}

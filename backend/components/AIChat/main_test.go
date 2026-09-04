package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/MrKOcode/AiChatBot3.0backenddeploy/backend/components/AIChat/services"
)

type fakeStore struct {
	createID              string
	createErr             error
	createUserID          string
	listUserID            string
	listSince             time.Time
	listConversationsPage services.ListPage[services.Conversation]
	listConversationsErr  error
	listMessagesPage      services.ListPage[services.ChatMessage]
	listMessagesErr       error
	listMessagesLimit     int32
	listMessagesNewest    bool
	historyPage           services.ListPage[services.ChatMessage]
	historyErr            error
	getConversationErr    error
	putMessageErrAt       int
	putMessages           []services.ChatMessage
	deleteErr             error
	deletedUserID         string
	deletedConversationID string
	memory                services.ConversationMemory
	profile               services.StudentProfile
	putMemory             services.ConversationMemory
	putProfile            services.StudentProfile
	memoryErr             error
	profileErr            error
}

func (f *fakeStore) CreateConversation(ctx context.Context, userID, title string) (string, error) {
	f.createUserID = userID
	if f.createID == "" {
		f.createID = "conversation-1"
	}
	return f.createID, f.createErr
}

func (f *fakeStore) ListConversations(ctx context.Context, userID string, limit int32, nextToken string) (services.ListPage[services.Conversation], error) {
	f.listUserID = userID
	return f.listConversationsPage, f.listConversationsErr
}

func (f *fakeStore) PutMessage(ctx context.Context, m services.ChatMessage) error {
	f.putMessages = append(f.putMessages, m)
	if f.putMessageErrAt == len(f.putMessages) {
		return errors.New("put message failed")
	}
	return nil
}

func (f *fakeStore) ListMessages(ctx context.Context, conversationID string, limit int32, nextToken string, newestFirst bool) (services.ListPage[services.ChatMessage], error) {
	f.listMessagesLimit = limit
	f.listMessagesNewest = newestFirst
	return f.listMessagesPage, f.listMessagesErr
}

func (f *fakeStore) DeleteConversationCascade(ctx context.Context, userID, conversationID string) error {
	f.deletedUserID = userID
	f.deletedConversationID = conversationID
	return f.deleteErr
}

func (f *fakeStore) ListUserMessagesSince(ctx context.Context, userID string, since time.Time, limit int32, nextToken string) (services.ListPage[services.ChatMessage], error) {
	f.listUserID = userID
	f.listSince = since
	if f.historyPage.Items == nil && f.historyErr == nil {
		now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
		f.historyPage.Items = []services.ChatMessage{
			{ConversationID: "conversation-1", UserID: userID, Role: "user", Content: "What is photosynthesis?", CreatedAt: now},
			{ConversationID: "conversation-1", UserID: userID, Role: "chatbot", Content: "Photosynthesis converts light into chemical energy.", CreatedAt: now.Add(time.Second)},
		}
	}
	return f.historyPage, f.historyErr
}

func (f *fakeStore) GetConversation(ctx context.Context, userID, conversationID string) (services.Conversation, error) {
	if f.getConversationErr != nil {
		return services.Conversation{}, f.getConversationErr
	}
	return services.Conversation{ID: conversationID, UserID: userID}, nil
}

func (f *fakeStore) GetConversationMemory(context.Context, string) (services.ConversationMemory, error) {
	return f.memory, f.memoryErr
}
func (f *fakeStore) PutConversationMemory(_ context.Context, memory services.ConversationMemory) error {
	f.putMemory = memory
	return f.memoryErr
}
func (f *fakeStore) GetStudentProfile(context.Context, string) (services.StudentProfile, error) {
	return f.profile, f.profileErr
}
func (f *fakeStore) PutStudentProfile(_ context.Context, profile services.StudentProfile) error {
	f.putProfile = profile
	return f.profileErr
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
	if len(store.putMessages) != 0 {
		t.Fatalf("expected no messages to be written before ownership check, wrote %d", len(store.putMessages))
	}
}

func TestHandlerRoutesAndNotFound(t *testing.T) {
	store := &fakeStore{}
	withFakeStore(t, store)
	t.Setenv("API_STAGE_NAME", "Test")

	tests := []struct {
		name   string
		req    events.APIGatewayProxyRequest
		status int
	}{
		{name: "preflight", req: events.APIGatewayProxyRequest{HTTPMethod: "OPTIONS"}, status: 200},
		{name: "fetch conversations", req: requestWithAuth("GET", "/Test/api/AIchat/conversations"), status: 200},
		{name: "history", req: requestWithAuth("GET", "/Test/api/AIchat/history"), status: 200},
		{name: "create", req: requestWithAuth("POST", "/Test/api/AIchat/conversations"), status: 200},
		{name: "not found", req: requestWithAuth("PATCH", "/Test/api/AIchat/nope"), status: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := handler(tt.req)
			if err != nil || resp.StatusCode != tt.status {
				t.Fatalf("handler() = %#v, err=%v; want status %d", resp, err, tt.status)
			}
		})
	}
}

func TestGetAuthInfo(t *testing.T) {
	tests := []struct {
		name    string
		req     events.APIGatewayProxyRequest
		wantErr bool
		admin   bool
		groups  int
	}{
		{name: "missing claims", req: events.APIGatewayProxyRequest{}, wantErr: true},
		{name: "invalid claims", req: events.APIGatewayProxyRequest{RequestContext: events.APIGatewayProxyRequestContext{Authorizer: map[string]interface{}{"claims": "bad"}}}, wantErr: true},
		{name: "missing sub", req: authRequest("", nil), wantErr: true},
		{name: "string groups", req: authRequest("u1", " students, admins "), admin: true, groups: 2},
		{name: "array groups", req: authRequest("u1", []interface{}{"students", 2, "admins"}), admin: true, groups: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAuthInfo(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getAuthInfo() err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && (got.IsAdmin != tt.admin || len(got.Groups) != tt.groups) {
				t.Fatalf("unexpected auth info: %+v", got)
			}
		})
	}
}

func TestResolveEffectiveUserID(t *testing.T) {
	req := authRequest("student-1", nil)
	if got, err := resolveEffectiveUserID(req, AuthInfo{Sub: "student-1"}); err != nil || got != "student-1" {
		t.Fatalf("own user resolution = %q, %v", got, err)
	}
	req.QueryStringParameters["targetUserId"] = "student-2"
	if _, err := resolveEffectiveUserID(req, AuthInfo{Sub: "student-1"}); err == nil {
		t.Fatal("expected non-admin target override to fail")
	}
	if got, err := resolveEffectiveUserID(req, AuthInfo{Sub: "admin-1", IsAdmin: true}); err != nil || got != "student-2" {
		t.Fatalf("admin target resolution = %q, %v", got, err)
	}
}

func TestCreateConversation(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := &fakeStore{}
		withFakeStore(t, store)
		resp, _ := lambdaCreateConversation(authRequest("student-1", nil))
		if resp.StatusCode != 200 || store.createUserID != "student-1" || len(store.putMessages) != 1 {
			t.Fatalf("unexpected response/store: %#v, %+v", resp, store)
		}
		if store.putMessages[0].Role != "chatbot" || store.putMessages[0].ConversationID != "conversation-1" {
			t.Fatalf("unexpected greeting: %+v", store.putMessages[0])
		}
	})
	t.Run("create failure", func(t *testing.T) {
		store := &fakeStore{createErr: errors.New("database unavailable")}
		withFakeStore(t, store)
		resp, _ := lambdaCreateConversation(authRequest("student-1", nil))
		if resp.StatusCode != 500 {
			t.Fatalf("got status %d", resp.StatusCode)
		}
	})
	t.Run("greeting failure", func(t *testing.T) {
		store := &fakeStore{putMessageErrAt: 1}
		withFakeStore(t, store)
		resp, _ := lambdaCreateConversation(authRequest("student-1", nil))
		if resp.StatusCode != 500 {
			t.Fatalf("got status %d", resp.StatusCode)
		}
	})
}

func TestFetchConversations(t *testing.T) {
	store := &fakeStore{listConversationsPage: services.ListPage[services.Conversation]{
		Items: []services.Conversation{{ID: "conversation-1", UserID: "student-1"}},
	}}
	withFakeStore(t, store)
	resp, _ := lambdaFetchConversations(authRequest("student-1", nil))
	if resp.StatusCode != 200 || !strings.Contains(resp.Body, "conversation-1") {
		t.Fatalf("unexpected response: %#v", resp)
	}

	store.listConversationsErr = errors.New("query failed")
	resp, _ = lambdaFetchConversations(authRequest("student-1", nil))
	if resp.StatusCode != 500 {
		t.Fatalf("got status %d", resp.StatusCode)
	}
}

func TestSendMessageValidationAndSuccess(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: "{"},
		{name: "missing conversation", body: `{"message":{"content":"hello"}}`},
		{name: "blank content", body: `{"message":{"conversationId":"conversation-1","content":"  "}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			withFakeStore(t, store)
			req := authRequest("student-1", nil)
			req.Body = tt.body
			resp, _ := lambdaSendMessage(req)
			if resp.StatusCode != 400 {
				t.Fatalf("got status %d, body %s", resp.StatusCode, resp.Body)
			}
		})
	}

	t.Run("success", func(t *testing.T) {
		store := &fakeStore{listMessagesPage: services.ListPage[services.ChatMessage]{Items: []services.ChatMessage{
			{Role: "chatbot", Content: "Previous answer"},
			{Role: "user", Content: "Previous question"},
		}}}
		withFakeStore(t, store)
		previous := getChatResponse
		getChatResponse = func(messages []services.Message) (string, error) {
			want := []services.Message{
				{Role: "user", Content: "Previous question"},
				{Role: "assistant", Content: "Previous answer"},
				{Role: "user", Content: "hello"},
			}
			if len(messages) != len(want) {
				t.Fatalf("AI received %+v", messages)
			}
			for i := range want {
				if messages[i] != want[i] {
					t.Fatalf("AI message %d = %+v, want %+v", i, messages[i], want[i])
				}
			}
			return "Hi there", nil
		}
		t.Cleanup(func() { getChatResponse = previous })
		req := authRequest("student-1", nil)
		req.Body = `{"message":{"conversationId":"conversation-1","content":"hello"}}`
		resp, _ := lambdaSendMessage(req)
		if resp.StatusCode != 200 || len(store.putMessages) != 2 {
			t.Fatalf("unexpected response/store: %#v, %+v", resp, store.putMessages)
		}
		if store.putMessages[0].Role != "user" || store.putMessages[1].Role != "chatbot" {
			t.Fatalf("unexpected roles: %+v", store.putMessages)
		}
		if store.listMessagesLimit != recentMessageQueryLimit || !store.listMessagesNewest {
			t.Fatalf("unexpected history query: limit=%d newest=%v", store.listMessagesLimit, store.listMessagesNewest)
		}
	})

	t.Run("AI failure", func(t *testing.T) {
		store := &fakeStore{}
		withFakeStore(t, store)
		previous := getChatResponse
		getChatResponse = func([]services.Message) (string, error) { return "", errors.New("AI unavailable") }
		t.Cleanup(func() { getChatResponse = previous })
		req := authRequest("student-1", nil)
		req.Body = `{"message":{"conversationId":"conversation-1","content":"hello"}}`
		resp, _ := lambdaSendMessage(req)
		if resp.StatusCode != 500 || len(store.putMessages) != 1 {
			t.Fatalf("unexpected response/store: %#v, %+v", resp, store.putMessages)
		}
	})

	t.Run("history failure", func(t *testing.T) {
		store := &fakeStore{listMessagesErr: errors.New("query failed")}
		withFakeStore(t, store)
		req := authRequest("student-1", nil)
		req.Body = `{"message":{"conversationId":"conversation-1","content":"hello"}}`
		resp, _ := lambdaSendMessage(req)
		if resp.StatusCode != 500 || len(store.putMessages) != 0 {
			t.Fatalf("unexpected response/store: %#v, %+v", resp, store.putMessages)
		}
	})
}

func TestOpenAIMessagesFiltersUnsupportedHistory(t *testing.T) {
	got := openAIMessages([]services.ChatMessage{
		{Role: "tool", Content: "ignore me"},
		{Role: "chatbot", Content: "  answer  "},
		{Role: "user", Content: "   "},
	}, "current", services.StudentProfile{}, services.ConversationMemory{})
	want := []services.Message{
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "current"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestThreeLayerContextAndTokenBudget(t *testing.T) {
	now := time.Now().UTC()
	recent := []services.ChatMessage{
		{Role: "chatbot", Content: "new answer", CreatedAt: now, TokenEstimate: 2},
		{Role: "user", Content: "new question", CreatedAt: now.Add(-time.Second), TokenEstimate: 2},
		{Role: "user", Content: "too old", CreatedAt: now.Add(-2 * time.Second), TokenEstimate: 50},
	}
	selected := selectRecentMessages(recent, 4)
	if len(selected) != 2 {
		t.Fatalf("selected %d messages, want 2", len(selected))
	}
	profile := services.StudentProfile{UserID: "u1", Goals: []string{"learn algebra"}}
	memory := services.ConversationMemory{Summary: "Previously studied factoring."}
	messages := openAIMessages(selected, "help me continue", profile, memory)
	if len(messages) != 4 || messages[0].Role != "system" || !strings.Contains(messages[0].Content, "learn algebra") || !strings.Contains(messages[0].Content, "factoring") {
		t.Fatalf("unexpected layered context: %+v", messages)
	}
	if messages[1].Content != "new question" || messages[2].Content != "new answer" || messages[3].Content != "help me continue" {
		t.Fatalf("unexpected message order: %+v", messages)
	}
}

func TestMessagesAfterAndMemoryThreshold(t *testing.T) {
	now := time.Now().UTC()
	messages := []services.ChatMessage{{CreatedAt: now}, {CreatedAt: now.Add(time.Second)}}
	if got := messagesAfter(messages, now); len(got) != 1 || !got[0].CreatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("unexpected messages after marker: %+v", got)
	}
	large := []services.ChatMessage{{Content: strings.Repeat("x", memoryTokenThreshold*3)}}
	if !shouldRefreshMemory(large) || shouldRefreshMemory([]services.ChatMessage{{Content: "short"}}) {
		t.Fatal("unexpected memory refresh threshold")
	}
}

func TestRefreshMemoryPersistsSummaryAndProfile(t *testing.T) {
	store := &fakeStore{}
	withFakeStore(t, store)
	previous := generateMemory
	generateMemory = func(summary string, profile services.StudentProfile, messages []services.ChatMessage) (services.MemoryUpdate, error) {
		if summary != "old" || len(messages) != 2 || messages[0].Content != "first" {
			t.Fatalf("unexpected summarization input: %q %+v", summary, messages)
		}
		return services.MemoryUpdate{Summary: "updated", Courses: []string{"Math"}, Strengths: []string{"factoring"}}, nil
	}
	t.Cleanup(func() { generateMemory = previous })
	now := time.Now().UTC()
	err := refreshMemory(context.Background(), "u1", services.ConversationMemory{ConversationID: "c1", Summary: "old", Version: 2}, services.StudentProfile{UserID: "u1"}, []services.ChatMessage{
		{Content: "second", CreatedAt: now.Add(time.Second)}, {Content: "first", CreatedAt: now},
	})
	if err != nil || store.putMemory.Summary != "updated" || store.putMemory.Version != 3 || store.putProfile.Courses[0] != "Math" {
		t.Fatalf("memory was not persisted: err=%v memory=%+v profile=%+v", err, store.putMemory, store.putProfile)
	}
}

func TestDeleteConversation(t *testing.T) {
	t.Setenv("API_STAGE_NAME", "Prod")
	tests := []struct {
		name       string
		path       string
		store      *fakeStore
		wantStatus int
	}{
		{name: "missing id", path: "/Prod/api/AIchat/conversations/", store: &fakeStore{}, wantStatus: 400},
		{name: "not owned", path: "/Prod/api/AIchat/conversations/c1", store: &fakeStore{getConversationErr: errors.New("missing")}, wantStatus: 403},
		{name: "delete failure", path: "/Prod/api/AIchat/conversations/c1", store: &fakeStore{deleteErr: errors.New("delete failed")}, wantStatus: 500},
		{name: "success", path: "/Prod/api/AIchat/conversations/c1", store: &fakeStore{}, wantStatus: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeStore(t, tt.store)
			req := authRequest("student-1", nil)
			req.Path = tt.path
			resp, _ := lambdaDeleteConversation(req)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("got status %d, body %s", resp.StatusCode, resp.Body)
			}
		})
	}
}

func TestFetchMessages(t *testing.T) {
	t.Setenv("API_STAGE_NAME", "Prod")
	tests := []struct {
		name       string
		path       string
		store      *fakeStore
		wantStatus int
	}{
		{name: "invalid path", path: "/Prod/api/AIchat/messages", store: &fakeStore{}, wantStatus: 400},
		{name: "not owned", path: "/Prod/api/AIchat/conversations/c1/messages", store: &fakeStore{getConversationErr: errors.New("missing")}, wantStatus: 403},
		{name: "query failure", path: "/Prod/api/AIchat/conversations/c1/messages", store: &fakeStore{listMessagesErr: errors.New("query failed")}, wantStatus: 500},
		{name: "success", path: "/Prod/api/AIchat/conversations/c1/messages", store: &fakeStore{listMessagesPage: services.ListPage[services.ChatMessage]{Items: []services.ChatMessage{{ID: "m1"}}, NextToken: "next"}}, wantStatus: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeStore(t, tt.store)
			req := authRequest("student-1", nil)
			req.Path = tt.path
			resp, _ := lambdaFetchMessages(req)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("got status %d, body %s", resp.StatusCode, resp.Body)
			}
			if tt.name == "success" && (!strings.Contains(resp.Body, `"hasMore":true`) || !strings.Contains(resp.Body, `"m1"`)) {
				t.Fatalf("unexpected success body: %s", resp.Body)
			}
		})
	}
}

func TestFetchHistoryPairsAndStopsAtFive(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var items []services.ChatMessage
	items = append(items, services.ChatMessage{ConversationID: "ignore", Role: "chatbot", Content: "orphan", CreatedAt: now})
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("c%d", i)
		items = append(items,
			services.ChatMessage{ConversationID: id, Role: "user", Content: "question", CreatedAt: now.Add(time.Duration(i+1) * time.Second)},
			services.ChatMessage{ConversationID: id, Role: "chatbot", Content: "answer", CreatedAt: now.Add(time.Duration(i+1)*time.Second + time.Millisecond)},
		)
	}
	store := &fakeStore{historyPage: services.ListPage[services.ChatMessage]{Items: items}}
	withFakeStore(t, store)
	resp, _ := lambdaFetchChatHistory(authRequest("student-1", nil))
	var body struct {
		History []map[string]string `json:"history"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.History) != 5 {
		t.Fatalf("got %d history entries", len(body.History))
	}
}

func TestResponseHelpersAndEnvironmentDefaults(t *testing.T) {
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("API_STAGE_NAME", "")
	if allowedOrigin() == "" || apiStageName() != "Prod" {
		t.Fatal("unexpected defaults")
	}
	t.Setenv("ALLOWED_ORIGIN", "https://example.com")
	t.Setenv("API_STAGE_NAME", "Dev")
	resp := errorResponse(418, "teapot")
	if resp.StatusCode != 418 || resp.Headers["Access-Control-Allow-Origin"] != "https://example.com" {
		t.Fatalf("unexpected error response: %#v", resp)
	}
	if apiStageName() != "Dev" {
		t.Fatal("stage override ignored")
	}
	if len(generateULID()) != 26 {
		t.Fatal("unexpected ULID")
	}
}

func requestWithAuth(method, path string) events.APIGatewayProxyRequest {
	req := authRequest("student-1", nil)
	req.HTTPMethod = method
	req.Path = path
	return req
}

package services

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeDynamo struct {
	putInputs    []*ddb.PutItemInput
	putErr       error
	queryOutputs []*ddb.QueryOutput
	queryErr     error
	queryCalls   int
	batchInputs  []*ddb.BatchWriteItemInput
	batchErr     error
	deleteInput  *ddb.DeleteItemInput
	deleteErr    error
	getOutput    *ddb.GetItemOutput
	getErr       error
}

func (f *fakeDynamo) PutItem(_ context.Context, in *ddb.PutItemInput, _ ...func(*ddb.Options)) (*ddb.PutItemOutput, error) {
	f.putInputs = append(f.putInputs, in)
	return &ddb.PutItemOutput{}, f.putErr
}
func (f *fakeDynamo) Query(_ context.Context, _ *ddb.QueryInput, _ ...func(*ddb.Options)) (*ddb.QueryOutput, error) {
	f.queryCalls++
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	if len(f.queryOutputs) == 0 {
		return &ddb.QueryOutput{}, nil
	}
	i := f.queryCalls - 1
	if i >= len(f.queryOutputs) {
		i = len(f.queryOutputs) - 1
	}
	return f.queryOutputs[i], nil
}
func (f *fakeDynamo) BatchWriteItem(_ context.Context, in *ddb.BatchWriteItemInput, _ ...func(*ddb.Options)) (*ddb.BatchWriteItemOutput, error) {
	f.batchInputs = append(f.batchInputs, in)
	return &ddb.BatchWriteItemOutput{}, f.batchErr
}
func (f *fakeDynamo) DeleteItem(_ context.Context, in *ddb.DeleteItemInput, _ ...func(*ddb.Options)) (*ddb.DeleteItemOutput, error) {
	f.deleteInput = in
	return &ddb.DeleteItemOutput{}, f.deleteErr
}
func (f *fakeDynamo) GetItem(_ context.Context, _ *ddb.GetItemInput, _ ...func(*ddb.Options)) (*ddb.GetItemOutput, error) {
	if f.getOutput == nil {
		f.getOutput = &ddb.GetItemOutput{}
	}
	return f.getOutput, f.getErr
}

func testDAL(client dynamoAPI) *dynamoDAL   { return &dynamoDAL{client: client, table: "table"} }
func avs(value string) types.AttributeValue { return &types.AttributeValueMemberS{Value: value} }

func conversationItem(id, user string, created time.Time) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": avs(pkUser(user)), "SK": avs(skConv(id)), "conversationId": avs(id),
		"userId": avs(user), "title": avs("Title"), "createdAt": avs(created.Format(time.RFC3339Nano)),
	}
}
func messageItem(id, conversationID, userID, role, content string, created time.Time) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": avs(pkConv(conversationID)), "SK": avs(skMsg(created, id)),
		"GSI1SK": avs(gsi1sk(created, conversationID, id)), "conversationId": avs(conversationID),
		"userId": avs(userID), "role": avs(role), "content": avs(content),
		"createdAt": avs(created.Format(time.RFC3339Nano)),
	}
}

func TestKeyAndValueHelpers(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 1, 2, 3, time.UTC)
	if pkUser("u") != "USER#u" || skConv("c") != "CONV#c" || pkConv("c") != "CONV#c" {
		t.Fatal("unexpected key prefix")
	}
	if !strings.Contains(skMsg(now, "m"), "#m") || gsi1pkUser("u") != "USER#u" ||
		!strings.Contains(gsi1sk(now, "c", "m"), "#CONV#c#MSG#m") {
		t.Fatal("unexpected message/GSI key")
	}
	if toEpochMs(now) != "1785412862000" {
		t.Fatalf("unexpected epoch: %s", toEpochMs(now))
	}
	if parseMessageID(skMsg(now, "message-1")) != "message-1" ||
		parseMessageID(gsi1sk(now, "c", "message-2")) != "message-2" ||
		parseMessageID("invalid") != "" {
		t.Fatal("message ID parsing failed")
	}
	if !parseTime(now.Format(time.RFC3339Nano)).Equal(now) || !parseTime("").IsZero() || !parseTime("bad").IsZero() {
		t.Fatal("time parsing failed")
	}
	if attrS(map[string]types.AttributeValue{"s": avs("value")}, "s") != "value" ||
		attrS(map[string]types.AttributeValue{"n": &types.AttributeValueMemberN{Value: "1"}}, "n") != "" {
		t.Fatal("attribute conversion failed")
	}
}

func TestPaginationTokenRoundTripAndErrors(t *testing.T) {
	if token, err := encodeLEK(nil); err != nil || token != "" {
		t.Fatalf("nil token = %q, %v", token, err)
	}
	key := map[string]types.AttributeValue{"PK": avs("USER#u")}
	token, err := encodeLEK(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeLEK(token)
	if err != nil || attrS(got, "PK") != "USER#u" {
		t.Fatalf("round trip = %#v, %v", got, err)
	}
	if got, err := decodeLEK(""); err != nil || got != nil {
		t.Fatalf("empty decode = %#v, %v", got, err)
	}
	if _, err := decodeLEK("%%%"); err == nil {
		t.Fatal("expected base64 error")
	}
	if _, err := decodeLEK(base64.StdEncoding.EncodeToString([]byte("{"))); err == nil {
		t.Fatal("expected JSON error")
	}
}

func TestInitDALRequiresTable(t *testing.T) {
	t.Setenv("TABLE_NAME", "")
	if err := InitDAL(); err == nil {
		t.Fatal("expected missing table error")
	}
}

func TestCreateConversationAndPutMessage(t *testing.T) {
	client := &fakeDynamo{}
	dal := testDAL(client)
	id, err := dal.CreateConversation(context.Background(), "u1", "Study")
	if err != nil || len(id) != 26 || len(client.putInputs) != 1 {
		t.Fatalf("create = %q, %v, inputs=%d", id, err, len(client.putInputs))
	}
	if attrS(client.putInputs[0].Item, "userId") != "u1" || client.putInputs[0].ConditionExpression == nil {
		t.Fatalf("unexpected create input: %#v", client.putInputs[0])
	}
	now := time.Now().UTC()
	err = dal.PutMessage(context.Background(), ChatMessage{
		ID: "m1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "hello", CreatedAt: now,
	})
	if err != nil || len(client.putInputs) != 2 || attrS(client.putInputs[1].Item, "GSI1PK") != "USER#u1" {
		t.Fatalf("put message failed: err=%v item=%#v", err, client.putInputs)
	}
	client.putErr = errors.New("write failed")
	if _, err := dal.CreateConversation(context.Background(), "u", "x"); err == nil {
		t.Fatal("expected create error")
	}
	if err := dal.PutMessage(context.Background(), ChatMessage{CreatedAt: now}); err == nil {
		t.Fatal("expected put error")
	}
}

func TestListConversations(t *testing.T) {
	now := time.Now().UTC()
	client := &fakeDynamo{queryOutputs: []*ddb.QueryOutput{{
		Items:            []map[string]types.AttributeValue{conversationItem("c1", "u1", now)},
		LastEvaluatedKey: map[string]types.AttributeValue{"PK": avs("next")},
	}}}
	page, err := testDAL(client).ListConversations(context.Background(), "u1", 20, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "c1" || page.NextToken == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if _, err := testDAL(&fakeDynamo{queryErr: errors.New("bad")}).ListConversations(context.Background(), "u", 1, ""); err == nil {
		t.Fatal("expected query error")
	}
	if _, err := testDAL(&fakeDynamo{}).ListConversations(context.Background(), "u", 1, "bad"); err == nil {
		t.Fatal("expected token error")
	}
}

func TestListMessages(t *testing.T) {
	now := time.Now().UTC()
	items := []map[string]types.AttributeValue{
		messageItem("later", "c1", "u1", "chatbot", "b", now.Add(time.Minute)),
		messageItem("earlier", "c1", "u1", "user", "a", now),
	}
	page, err := testDAL(&fakeDynamo{queryOutputs: []*ddb.QueryOutput{{Items: items}}}).
		ListMessages(context.Background(), "c1", 10, "", false)
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != "earlier" || page.Items[1].ID != "later" {
		t.Fatalf("ascending page=%+v err=%v", page, err)
	}
	page, err = testDAL(&fakeDynamo{queryOutputs: []*ddb.QueryOutput{{Items: items}}}).
		ListMessages(context.Background(), "c1", 10, "", true)
	if err != nil || page.Items[0].ID != "later" {
		t.Fatalf("newest page=%+v err=%v", page, err)
	}
	if _, err := testDAL(&fakeDynamo{queryErr: errors.New("bad")}).ListMessages(context.Background(), "c", 1, "", false); err == nil {
		t.Fatal("expected query error")
	}
	if _, err := testDAL(&fakeDynamo{}).ListMessages(context.Background(), "c", 1, "bad", false); err == nil {
		t.Fatal("expected token error")
	}
}

func TestListUserMessagesSince(t *testing.T) {
	now := time.Now().UTC()
	client := &fakeDynamo{queryOutputs: []*ddb.QueryOutput{{Items: []map[string]types.AttributeValue{
		messageItem("m1", "c1", "u1", "user", "hello", now),
	}}}}
	page, err := testDAL(client).ListUserMessagesSince(context.Background(), "u1", now.Add(-time.Hour), 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "m1" || page.Items[0].ConversationID != "c1" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if _, err := testDAL(&fakeDynamo{queryErr: errors.New("bad")}).ListUserMessagesSince(context.Background(), "u", now, 1, ""); err == nil {
		t.Fatal("expected query error")
	}
	if _, err := testDAL(&fakeDynamo{}).ListUserMessagesSince(context.Background(), "u", now, 1, "bad"); err == nil {
		t.Fatal("expected token error")
	}
}

func TestGetConversation(t *testing.T) {
	now := time.Now().UTC()
	got, err := testDAL(&fakeDynamo{getOutput: &ddb.GetItemOutput{Item: conversationItem("c1", "u1", now)}}).
		GetConversation(context.Background(), "u1", "c1")
	if err != nil || got.ID != "c1" || got.UserID != "u1" || got.Title != "Title" {
		t.Fatalf("conversation=%+v err=%v", got, err)
	}
	if _, err := testDAL(&fakeDynamo{}).GetConversation(context.Background(), "u", "c"); err == nil {
		t.Fatal("expected not found")
	}
	if _, err := testDAL(&fakeDynamo{getErr: errors.New("read failed")}).GetConversation(context.Background(), "u", "c"); err == nil {
		t.Fatal("expected read error")
	}
}

func TestDeleteConversationCascade(t *testing.T) {
	var items []map[string]types.AttributeValue
	for i := 0; i < 30; i++ {
		items = append(items, map[string]types.AttributeValue{
			"PK": avs("CONV#c1"), "SK": avs("MSG#" + string(rune('a'+i))),
		})
	}
	client := &fakeDynamo{queryOutputs: []*ddb.QueryOutput{{Items: items}}}
	err := testDAL(client).DeleteConversationCascade(context.Background(), "u1", "c1")
	if err != nil || len(client.batchInputs) != 2 || client.deleteInput == nil {
		t.Fatalf("err=%v batches=%d delete=%#v", err, len(client.batchInputs), client.deleteInput)
	}
	if got := attrS(client.deleteInput.Key, "PK"); got != "USER#u1" {
		t.Fatalf("header PK=%q", got)
	}
	if err := testDAL(&fakeDynamo{queryErr: errors.New("query failed")}).DeleteConversationCascade(context.Background(), "u", "c"); err == nil {
		t.Fatal("expected query error")
	}
	if err := testDAL(&fakeDynamo{queryOutputs: []*ddb.QueryOutput{{Items: items}}, batchErr: errors.New("batch failed")}).
		DeleteConversationCascade(context.Background(), "u", "c"); err == nil {
		t.Fatal("expected batch error")
	}
	if err := testDAL(&fakeDynamo{deleteErr: errors.New("delete failed")}).
		DeleteConversationCascade(context.Background(), "u", "c"); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestGetChatGPTResponse(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "")
		if _, err := GetChatGPTResponse([]Message{{Role: "user", Content: "hello"}}); err == nil {
			t.Fatal("expected missing key error")
		}
	})
	tests := []struct {
		name, body string
		status     int
		want       string
		wantErr    bool
		checkInput bool
	}{
		{name: "success", status: 200, body: `{"choices":[{"message":{"role":"assistant","content":"answer"}}]}`, want: "answer", checkInput: true},
		{name: "API error", status: 429, body: `{"error":"limited"}`, wantErr: true},
		{name: "bad JSON", status: 200, body: `{`, wantErr: true},
		{name: "no choices", status: 200, body: `{"choices":[]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.checkInput {
					if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
						t.Errorf("unexpected request: %s %#v", r.Method, r.Header)
					}
					body, _ := io.ReadAll(r.Body)
					payload := string(body)
					if !strings.Contains(payload, `"role":"user","content":"question"`) ||
						!strings.Contains(payload, `"role":"assistant","content":"previous answer"`) {
						t.Errorf("unexpected payload: %s", body)
					}
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			oldURL, oldClient := chatGPTAPIURL, chatGPTHTTPClient
			chatGPTAPIURL, chatGPTHTTPClient = server.URL, server.Client()
			t.Cleanup(func() { chatGPTAPIURL, chatGPTHTTPClient = oldURL, oldClient })
			t.Setenv("OPENAI_API_KEY", "test-key")
			got, err := GetChatGPTResponse([]Message{
				{Role: "assistant", Content: "previous answer"},
				{Role: "user", Content: "question"},
			})
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("response=%q err=%v", got, err)
			}
		})
	}
	t.Run("transport error", func(t *testing.T) {
		oldURL, oldClient := chatGPTAPIURL, chatGPTHTTPClient
		chatGPTAPIURL = "http://127.0.0.1:1"
		chatGPTHTTPClient = &http.Client{Timeout: 100 * time.Millisecond}
		t.Cleanup(func() { chatGPTAPIURL, chatGPTHTTPClient = oldURL, oldClient })
		t.Setenv("OPENAI_API_KEY", "test-key")
		if _, err := GetChatGPTResponse([]Message{{Role: "user", Content: "question"}}); err == nil {
			t.Fatal("expected transport error")
		}
	})
}

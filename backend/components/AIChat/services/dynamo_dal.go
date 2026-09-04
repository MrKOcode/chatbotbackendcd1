package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/oklog/ulid/v2"
)

const (
	entityConversation = "Conversation"
	entityMessage      = "Message"
	entityMemory       = "ConversationMemory"
	entityProfile      = "StudentProfile"
)

// Key helpers
func pkUser(userID string) string         { return "USER#" + userID }
func skConv(conversationID string) string { return "CONV#" + conversationID }
func pkConv(conversationID string) string { return "CONV#" + conversationID }
func skMsg(ts time.Time, messageID string) string {
	return "MSG#" + ts.UTC().Format(time.RFC3339Nano) + "#" + messageID
}
func skMemory() string                { return "MEMORY" }
func skProfile() string               { return "LEARNING_PROFILE" }
func gsi1pkUser(userID string) string { return "USER#" + userID }
func gsi1sk(ts time.Time, conversationID, messageID string) string {
	return "TS#" + ts.UTC().Format(time.RFC3339Nano) + "#CONV#" + conversationID + "#MSG#" + messageID
}

type dynamoDAL struct {
	client dynamoAPI
	table  string
}

type dynamoAPI interface {
	PutItem(context.Context, *ddb.PutItemInput, ...func(*ddb.Options)) (*ddb.PutItemOutput, error)
	Query(context.Context, *ddb.QueryInput, ...func(*ddb.Options)) (*ddb.QueryOutput, error)
	BatchWriteItem(context.Context, *ddb.BatchWriteItemInput, ...func(*ddb.Options)) (*ddb.BatchWriteItemOutput, error)
	DeleteItem(context.Context, *ddb.DeleteItemInput, ...func(*ddb.Options)) (*ddb.DeleteItemOutput, error)
	GetItem(context.Context, *ddb.GetItemInput, ...func(*ddb.Options)) (*ddb.GetItemOutput, error)
}

// Global, used by your handlers/services.
var Store DAL

// Call once during cold start (e.g., in main.init or first handler call)
func InitDAL() error {
	table := os.Getenv("TABLE_NAME")
	if table == "" {
		return errors.New("TABLE_NAME env var is required")
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return err
	}
	Store = &dynamoDAL{
		client: ddb.NewFromConfig(cfg),
		table:  table,
	}
	return nil
}

// ---------- Key encoding for NextToken ----------

func encodeLEK(lek map[string]types.AttributeValue) (string, error) {
	if lek == nil {
		return "", nil
	}
	values := make(map[string]string, len(lek))
	for key, value := range lek {
		stringValue, ok := value.(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("unsupported pagination key type for %s", key)
		}
		values[key] = stringValue.Value
	}
	b, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func decodeLEK(token string) (map[string]types.AttributeValue, error) {
	if token == "" {
		return nil, nil
	}
	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if err := json.Unmarshal(b, &values); err != nil {
		return nil, err
	}
	m := make(map[string]types.AttributeValue, len(values))
	for key, value := range values {
		m[key] = &types.AttributeValueMemberS{Value: value}
	}
	return m, nil
}

// ---------- DAL methods ----------

func (d *dynamoDAL) CreateConversation(ctx context.Context, userID, title string) (string, error) {
	id := ulid.Make().String()
	now := time.Now().UTC()

	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: pkUser(userID)},
		"SK":             &types.AttributeValueMemberS{Value: skConv(id)},
		"entityType":     &types.AttributeValueMemberS{Value: entityConversation},
		"conversationId": &types.AttributeValueMemberS{Value: id},
		"userId":         &types.AttributeValueMemberS{Value: userID},
		"title":          &types.AttributeValueMemberS{Value: title},
		"createdAt":      &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
	}

	_, err := d.client.PutItem(ctx, &ddb.PutItemInput{
		TableName:           aws.String(d.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	return id, err
}

func (d *dynamoDAL) ListConversations(ctx context.Context, userID string, limit int32, nextToken string) (ListPage[Conversation], error) {
	lek, err := decodeLEK(nextToken)
	if err != nil {
		return ListPage[Conversation]{}, err
	}

	out, err := d.client.Query(ctx, &ddb.QueryInput{
		TableName:              aws.String(d.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :conv)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: pkUser(userID)},
			":conv": &types.AttributeValueMemberS{Value: "CONV#"},
		},
		Limit:             aws.Int32(limit),
		ExclusiveStartKey: lek,
		ScanIndexForward:  aws.Bool(false), // newest first if you maintain lastMessageAt later
	})
	if err != nil {
		return ListPage[Conversation]{}, err
	}

	var items []Conversation
	for _, it := range out.Items {
		items = append(items, Conversation{
			ID:        attrS(it, "conversationId"),
			UserID:    attrS(it, "userId"),
			Title:     attrS(it, "title"),
			CreatedAt: parseTime(attrS(it, "createdAt")),
		})
	}
	token, _ := encodeLEK(out.LastEvaluatedKey)
	return ListPage[Conversation]{Items: items, NextToken: token}, nil
}

func (d *dynamoDAL) PutMessage(ctx context.Context, m ChatMessage) error {
	ts := m.CreatedAt.UTC()
	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: pkConv(m.ConversationID)},
		"SK":             &types.AttributeValueMemberS{Value: skMsg(ts, m.ID)},
		"entityType":     &types.AttributeValueMemberS{Value: entityMessage},
		"conversationId": &types.AttributeValueMemberS{Value: m.ConversationID},
		"userId":         &types.AttributeValueMemberS{Value: m.UserID},
		"role":           &types.AttributeValueMemberS{Value: m.Role},
		"content":        &types.AttributeValueMemberS{Value: m.Content},
		"createdAt":      &types.AttributeValueMemberS{Value: ts.Format(time.RFC3339Nano)},
		"epochMs":        &types.AttributeValueMemberN{Value: toEpochMs(ts)},
		"tokenEstimate":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", m.TokenEstimate)},
		// GSI1 for user-time queries:
		"GSI1PK": &types.AttributeValueMemberS{Value: gsi1pkUser(m.UserID)},
		"GSI1SK": &types.AttributeValueMemberS{Value: gsi1sk(ts, m.ConversationID, m.ID)},
	}

	_, err := d.client.PutItem(ctx, &ddb.PutItemInput{
		TableName: aws.String(d.table),
		Item:      item,
	})
	return err
}

func (d *dynamoDAL) ListMessages(ctx context.Context, conversationID string, limit int32, nextToken string, newestFirst bool) (ListPage[ChatMessage], error) {
	lek, err := decodeLEK(nextToken)
	if err != nil {
		return ListPage[ChatMessage]{}, err
	}

	out, err := d.client.Query(ctx, &ddb.QueryInput{
		TableName:              aws.String(d.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :msg)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":  &types.AttributeValueMemberS{Value: pkConv(conversationID)},
			":msg": &types.AttributeValueMemberS{Value: "MSG#"},
		},
		Limit:             aws.Int32(limit),
		ExclusiveStartKey: lek,
		ScanIndexForward:  aws.Bool(!newestFirst), // Dynamo ascending when true
	})
	if err != nil {
		return ListPage[ChatMessage]{}, err
	}

	var items []ChatMessage
	for _, it := range out.Items {
		items = append(items, ChatMessage{
			ID:             parseMessageID(attrS(it, "SK")),
			ConversationID: conversationID,
			UserID:         attrS(it, "userId"),
			Role:           attrS(it, "role"),
			Content:        attrS(it, "content"),
			CreatedAt:      parseTime(attrS(it, "createdAt")),
			TokenEstimate:  attrInt(it, "tokenEstimate"),
		})
	}
	// If newestFirst==true and Dynamo returned ascending (because ScanIndexForward=false already gives descending),
	// we’re good. If you ever switch to ascending, reverse here:
	if newestFirst {
		// already descending from query; keep as-is
	} else {
		// ensure ascending
		sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	}

	token, _ := encodeLEK(out.LastEvaluatedKey)
	return ListPage[ChatMessage]{Items: items, NextToken: token}, nil
}

func (d *dynamoDAL) DeleteConversationCascade(ctx context.Context, userID, conversationID string) error {
	// Query all PK=CONV#id and batch delete
	var lek map[string]types.AttributeValue
	for {
		out, err := d.client.Query(ctx, &ddb.QueryInput{
			TableName:              aws.String(d.table),
			KeyConditionExpression: aws.String("PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: pkConv(conversationID)},
			},
			ExclusiveStartKey: lek,
		})
		if err != nil {
			return err
		}

		if len(out.Items) > 0 {
			writes := make([]types.WriteRequest, 0, len(out.Items))
			for _, it := range out.Items {
				writes = append(writes, types.WriteRequest{
					DeleteRequest: &types.DeleteRequest{
						Key: map[string]types.AttributeValue{
							"PK": it["PK"],
							"SK": it["SK"],
						},
					},
				})
			}
			// BatchWriteItem in chunks of 25
			for i := 0; i < len(writes); i += 25 {
				end := i + 25
				if end > len(writes) {
					end = len(writes)
				}
				_, err := d.client.BatchWriteItem(ctx, &ddb.BatchWriteItemInput{
					RequestItems: map[string][]types.WriteRequest{d.table: writes[i:end]},
				})
				if err != nil {
					return err
				}
			}
		}

		if out.LastEvaluatedKey == nil {
			break
		}
		lek = out.LastEvaluatedKey
	}
	_, err := d.client.DeleteItem(ctx, &ddb.DeleteItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pkUser(userID)},
			"SK": &types.AttributeValueMemberS{Value: skConv(conversationID)},
		},
	})
	if err != nil {
		return err
	}
	// Note: To delete the header, you need userID; do this delete in handler where you have the userID.
	return nil
}

func (d *dynamoDAL) ListUserMessagesSince(ctx context.Context, userID string, since time.Time, limit int32, nextToken string) (ListPage[ChatMessage], error) {
	lek, err := decodeLEK(nextToken)
	if err != nil {
		return ListPage[ChatMessage]{}, err
	}

	out, err := d.client.Query(ctx, &ddb.QueryInput{
		TableName:              aws.String(d.table),
		IndexName:              aws.String("GSI1"), // create GSI1 (GSI1PK, GSI1SK)
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK >= :from"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: gsi1pkUser(userID)},
			":from": &types.AttributeValueMemberS{Value: "TS#" + since.UTC().Format(time.RFC3339Nano)},
		},
		Limit:             aws.Int32(limit),
		ExclusiveStartKey: lek,
		ScanIndexForward:  aws.Bool(true),
	})
	if err != nil {
		return ListPage[ChatMessage]{}, err
	}

	var items []ChatMessage
	for _, it := range out.Items {
		items = append(items, ChatMessage{
			ID:             parseMessageID(attrS(it, "GSI1SK")),
			ConversationID: attrS(it, "conversationId"),
			UserID:         attrS(it, "userId"),
			Role:           attrS(it, "role"),
			Content:        attrS(it, "content"),
			CreatedAt:      parseTime(attrS(it, "createdAt")),
			TokenEstimate:  attrInt(it, "tokenEstimate"),
		})
	}
	token, _ := encodeLEK(out.LastEvaluatedKey)
	return ListPage[ChatMessage]{Items: items, NextToken: token}, nil
}

// ---------- helpers ----------

func attrS(m map[string]types.AttributeValue, k string) string {
	if v, ok := m[k].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}
func attrInt(m map[string]types.AttributeValue, k string) int {
	if v, ok := m[k].(*types.AttributeValueMemberN); ok {
		var value int
		_, _ = fmt.Sscanf(v.Value, "%d", &value)
		return value
	}
	return 0
}

func attrStrings(m map[string]types.AttributeValue, k string) []string {
	if v, ok := m[k].(*types.AttributeValueMemberL); ok {
		result := make([]string, 0, len(v.Value))
		for _, item := range v.Value {
			if s, ok := item.(*types.AttributeValueMemberS); ok && strings.TrimSpace(s.Value) != "" {
				result = append(result, s.Value)
			}
		}
		return result
	}
	return nil
}

func stringList(values []string) types.AttributeValue {
	items := make([]types.AttributeValue, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			items = append(items, &types.AttributeValueMemberS{Value: value})
		}
	}
	return &types.AttributeValueMemberL{Value: items}
}
func toEpochMs(t time.Time) string {
	return fmt.Sprintf("%d", t.UnixMilli())
}
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
func parseMessageID(skOrGsi1sk string) string {
	if marker := strings.LastIndex(skOrGsi1sk, "#MSG#"); marker >= 0 {
		return skOrGsi1sk[marker+len("#MSG#"):]
	}
	if marker := strings.LastIndex(skOrGsi1sk, "#"); marker >= 0 {
		return skOrGsi1sk[marker+1:]
	}
	return ""
}

func (d *dynamoDAL) GetConversation(ctx context.Context, userID, conversationID string) (Conversation, error) {
	out, err := d.client.GetItem(ctx, &ddb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pkUser(userID)},
			"SK": &types.AttributeValueMemberS{Value: skConv(conversationID)},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return Conversation{}, err
	}
	if out.Item == nil || len(out.Item) == 0 {
		return Conversation{}, fmt.Errorf("conversation not found")
	}

	// optional: you can check entityType == "Conversation" if you want
	return Conversation{
		ID:        attrS(out.Item, "conversationId"),
		UserID:    attrS(out.Item, "userId"),
		Title:     attrS(out.Item, "title"),
		CreatedAt: parseTime(attrS(out.Item, "createdAt")),
	}, nil
}

func (d *dynamoDAL) GetConversationMemory(ctx context.Context, conversationID string) (ConversationMemory, error) {
	out, err := d.client.GetItem(ctx, &ddb.GetItemInput{
		TableName:      aws.String(d.table),
		Key:            map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: pkConv(conversationID)}, "SK": &types.AttributeValueMemberS{Value: skMemory()}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return ConversationMemory{}, err
	}
	if len(out.Item) == 0 {
		return ConversationMemory{ConversationID: conversationID}, nil
	}
	return ConversationMemory{
		ConversationID:    conversationID,
		Summary:           attrS(out.Item, "summary"),
		SummarizedThrough: parseTime(attrS(out.Item, "summarizedThrough")),
		Version:           attrInt(out.Item, "version"),
		UpdatedAt:         parseTime(attrS(out.Item, "updatedAt")),
	}, nil
}

func (d *dynamoDAL) PutConversationMemory(ctx context.Context, memory ConversationMemory) error {
	now := memory.UpdatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := d.client.PutItem(ctx, &ddb.PutItemInput{TableName: aws.String(d.table), Item: map[string]types.AttributeValue{
		"PK":                &types.AttributeValueMemberS{Value: pkConv(memory.ConversationID)},
		"SK":                &types.AttributeValueMemberS{Value: skMemory()},
		"entityType":        &types.AttributeValueMemberS{Value: entityMemory},
		"conversationId":    &types.AttributeValueMemberS{Value: memory.ConversationID},
		"summary":           &types.AttributeValueMemberS{Value: memory.Summary},
		"summarizedThrough": &types.AttributeValueMemberS{Value: memory.SummarizedThrough.UTC().Format(time.RFC3339Nano)},
		"version":           &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", memory.Version)},
		"updatedAt":         &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
	}})
	return err
}

func (d *dynamoDAL) GetStudentProfile(ctx context.Context, userID string) (StudentProfile, error) {
	out, err := d.client.GetItem(ctx, &ddb.GetItemInput{
		TableName:      aws.String(d.table),
		Key:            map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: pkUser(userID)}, "SK": &types.AttributeValueMemberS{Value: skProfile()}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return StudentProfile{}, err
	}
	if len(out.Item) == 0 {
		return StudentProfile{UserID: userID}, nil
	}
	return StudentProfile{UserID: userID, Courses: attrStrings(out.Item, "courses"), Goals: attrStrings(out.Item, "goals"), Strengths: attrStrings(out.Item, "strengths"), Misconceptions: attrStrings(out.Item, "misconceptions"), Preferences: attrStrings(out.Item, "preferences"), UpdatedAt: parseTime(attrS(out.Item, "updatedAt"))}, nil
}

func (d *dynamoDAL) PutStudentProfile(ctx context.Context, profile StudentProfile) error {
	now := profile.UpdatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := d.client.PutItem(ctx, &ddb.PutItemInput{TableName: aws.String(d.table), Item: map[string]types.AttributeValue{
		"PK":         &types.AttributeValueMemberS{Value: pkUser(profile.UserID)},
		"SK":         &types.AttributeValueMemberS{Value: skProfile()},
		"entityType": &types.AttributeValueMemberS{Value: entityProfile},
		"userId":     &types.AttributeValueMemberS{Value: profile.UserID},
		"courses":    stringList(profile.Courses), "goals": stringList(profile.Goals), "strengths": stringList(profile.Strengths),
		"misconceptions": stringList(profile.Misconceptions), "preferences": stringList(profile.Preferences),
		"updatedAt": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
	}})
	return err
}

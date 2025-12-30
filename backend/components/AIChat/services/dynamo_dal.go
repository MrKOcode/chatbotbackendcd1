package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
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
)

// Key helpers
func pkUser(userID string) string         { return "USER#" + userID }
func skConv(conversationID string) string { return "CONV#" + conversationID }
func pkConv(conversationID string) string { return "CONV#" + conversationID }
func skMsg(ts time.Time, messageID string) string {
	return "MSG#" + ts.UTC().Format(time.RFC3339Nano) + "#" + messageID
}
func gsi1pkUser(userID string) string { return "USER#" + userID }
func gsi1sk(ts time.Time, conversationID, messageID string) string {
	return "TS#" + ts.UTC().Format(time.RFC3339Nano) + "#CONV#" + conversationID + "#MSG#" + messageID
}

type dynamoDAL struct {
	client *ddb.Client
	table  string
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
	b, err := json.Marshal(lek)
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
	var m map[string]types.AttributeValue
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
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
func toEpochMs(t time.Time) string {
	return aws.ToString(aws.String((time.Duration(t.UnixNano() / 1e6)).String()))
}
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
func parseMessageID(skOrGsi1sk string) string {
	// sk:  MSG#<ts>#<id>
	// gsi: TS#<ts>#CONV#<cid>#MSG#<id>
	parts := []rune(skOrGsi1sk)
	_ = parts
	// simplest: not needed by callers right now; return ""
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

package services

import (
	"context"
	"time"
)

type Conversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
}

type ChatMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	UserID         string    `json:"userId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Generic paged list (Dynamo uses a "cursor" token, not offset)
type ListPage[T any] struct {
	Items     []T    `json:"items"`
	NextToken string `json:"nextToken,omitempty"` // base64-encoded LastEvaluatedKey
}

type DAL interface {
	CreateConversation(ctx context.Context, userID, title string) (string, error)
	ListConversations(ctx context.Context, userID string, limit int32, nextToken string) (ListPage[Conversation], error)
	PutMessage(ctx context.Context, m ChatMessage) error
	ListMessages(ctx context.Context, conversationID string, limit int32, nextToken string, newestFirst bool) (ListPage[ChatMessage], error)
	DeleteConversationCascade(ctx context.Context, userID, conversationID string) error
	ListUserMessagesSince(ctx context.Context, userID string, since time.Time, limit int32, nextToken string) (ListPage[ChatMessage], error)
	GetConversation(ctx context.Context, userID, conversationID string) (Conversation, error)
}

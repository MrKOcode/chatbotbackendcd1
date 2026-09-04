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
	TokenEstimate  int       `json:"tokenEstimate,omitempty"`
}

type ConversationMemory struct {
	ConversationID    string    `json:"conversationId"`
	Summary           string    `json:"summary"`
	SummarizedThrough time.Time `json:"summarizedThrough"`
	Version           int       `json:"version"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type StudentProfile struct {
	UserID         string    `json:"userId"`
	Courses        []string  `json:"courses,omitempty"`
	Goals          []string  `json:"goals,omitempty"`
	Strengths      []string  `json:"strengths,omitempty"`
	Misconceptions []string  `json:"misconceptions,omitempty"`
	Preferences    []string  `json:"preferences,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
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
	GetConversationMemory(ctx context.Context, conversationID string) (ConversationMemory, error)
	PutConversationMemory(ctx context.Context, memory ConversationMemory) error
	GetStudentProfile(ctx context.Context, userID string) (StudentProfile, error)
	PutStudentProfile(ctx context.Context, profile StudentProfile) error
}

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MrKOcode/AiChatBot3.0backenddeploy/backend/components/AIChat/services"
)

// Serialize individual storage operations, just as atomic item writes do.
// This deliberately does NOT make the whole read/generate/write workflow atomic.
type memoryRaceStore struct {
	fakeStore
	mu     sync.Mutex
	writes []services.ConversationMemory
}

func (s *memoryRaceStore) GetConversationMemory(context.Context, string) (services.ConversationMemory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.memory, nil
}

func (s *memoryRaceStore) PutConversationMemory(_ context.Context, m services.ConversationMemory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memory = m // Same last-writer-wins behavior as the current unconditional PutItem.
	s.writes = append(s.writes, m)
	return nil
}

func (s *memoryRaceStore) PutStudentProfile(_ context.Context, p services.StudentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile = p
	return nil
}

// This passing exercise asserts the CURRENT BUG, not the desired behavior.
// After adding optimistic locking, replace these expectations with conflict handling.
func TestDemoLostMemoryUpdate(t *testing.T) {
	store := &memoryRaceStore{fakeStore: fakeStore{
		memory: services.ConversationMemory{ConversationID: "demo", Summary: "Initial facts", Version: 2},
	}}
	previousStore, previousGenerator := services.Store, generateMemory
	services.Store = store
	t.Cleanup(func() { services.Store, generateMemory = previousStore, previousGenerator })

	// Hold BOTH requests inside generation until both have read version 2.
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	generateMemory = func(summary string, profile services.StudentProfile, messages []services.ChatMessage) (services.MemoryUpdate, error) {
		arrived <- struct{}{}
		<-release
		return services.MemoryUpdate{Summary: summary + "; " + messages[0].Content}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errs := make(chan error, 2)
	for _, fact := range []string{"A: studying algebra", "B: studying biology"} {
		go func(fact string) {
			memory, err := store.GetConversationMemory(ctx, "demo")
			if err == nil {
				err = refreshMemory(ctx, "student", memory, services.StudentProfile{UserID: "student"}, []services.ChatMessage{
					{Content: fact, CreatedAt: time.Now().UTC()},
				})
			}
			errs <- err
		}(fact)
	}
	ready := true
	for i := 0; i < 2; i++ {
		select {
		case <-arrived:
		case <-ctx.Done():
			ready = false
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if !ready {
		t.Fatal("requests did not reach the generation barrier")
	}
	if len(store.writes) != 2 || store.writes[0].Version != 3 || store.writes[1].Version != 3 {
		t.Fatalf("expected two writes of version 3: %+v", store.writes)
	}
	if store.writes[0].Summary == store.writes[1].Summary || store.memory.Summary != store.writes[1].Summary {
		t.Fatalf("expected the second distinct summary to overwrite the first: %+v", store.writes)
	}
	t.Logf("Both requests read version 2. First write: %+v", store.writes[0])
	t.Logf("Second write: %+v", store.writes[1])
	t.Logf("Final memory: version=%d summary=%q; the first request's new fact was lost", store.memory.Version, store.memory.Summary)
}

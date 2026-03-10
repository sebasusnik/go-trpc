// store.go — In-memory chat persistence (demo only).
//
// Messages are capped at 100 per room to limit memory usage on
// Koyeb's free tier (512MB). The store includes a pub/sub mechanism
// for broadcasting new messages to SSE subscribers.
package app

import (
	"sync"
	"time"
)

const maxMessagesPerRoom = 100

// ChatStore is a thread-safe in-memory store for rooms and messages.
type ChatStore struct {
	mu          sync.RWMutex
	rooms       map[string]Room
	messages    map[string][]Message // roomID -> messages
	subscribers map[string][]chan Message
	nextRoomID  int
	nextMsgID   int
}

// NewChatStore creates a store pre-populated with demo rooms.
func NewChatStore() *ChatStore {
	store := &ChatStore{
		rooms:       make(map[string]Room),
		messages:    make(map[string][]Message),
		subscribers: make(map[string][]chan Message),
		nextRoomID:  1,
		nextMsgID:   1,
	}
	store.seed()
	return store
}

func (s *ChatStore) seed() {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, name := range []string{"general", "random", "go-trpc"} {
		id := s.generateRoomID()
		s.rooms[id] = Room{ID: id, Name: name, CreatedAt: now}
		s.messages[id] = nil
	}
}

func (s *ChatStore) generateRoomID() string {
	id := "room-" + itoa(s.nextRoomID)
	s.nextRoomID++
	return id
}

func (s *ChatStore) generateMsgID() string {
	id := "msg-" + itoa(s.nextMsgID)
	s.nextMsgID++
	return id
}

// Subscribe returns a channel that receives new messages for a room.
// The caller must call Unsubscribe when done to avoid leaks.
func (s *ChatStore) Subscribe(roomID string) chan Message {
	ch := make(chan Message, 16)
	s.mu.Lock()
	s.subscribers[roomID] = append(s.subscribers[roomID], ch)
	s.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel for a room and closes it.
func (s *ChatStore) Unsubscribe(roomID string, ch chan Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.subscribers[roomID]
	for i, sub := range subs {
		if sub == ch {
			s.subscribers[roomID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// Broadcast sends a message to all subscribers of a room.
func (s *ChatStore) Broadcast(roomID string, msg Message) {
	s.mu.RLock()
	subs := make([]chan Message, len(s.subscribers[roomID]))
	copy(subs, s.subscribers[roomID])
	s.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
			// subscriber too slow, drop message
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

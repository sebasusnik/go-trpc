// handlers.go — tRPC procedure handlers.
//
// Each handler is a factory that takes a *ChatStore and returns a typed
// handler func(ctx, Input) (Output, error). The input/output types live
// in types.go — those are what `gotrpc generate` reads to produce the
// TypeScript AppRouter type.
package app

import (
	"context"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/codegen"
	"github.com/sebasusnik/go-trpc/pkg/errors"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// ListRooms returns all available chat rooms.
func ListRooms(store *ChatStore) func(ctx context.Context, input struct{}) (ListRoomsOutput, error) {
	return func(ctx context.Context, input struct{}) (ListRoomsOutput, error) {
		store.mu.RLock()
		defer store.mu.RUnlock()

		rooms := make([]Room, 0, len(store.rooms))
		for _, r := range store.rooms {
			rooms = append(rooms, r)
		}

		// Sort by ID (sequential = creation order)
		for i := 0; i < len(rooms); i++ {
			for j := i + 1; j < len(rooms); j++ {
				if rooms[i].ID > rooms[j].ID {
					rooms[i], rooms[j] = rooms[j], rooms[i]
				}
			}
		}

		return ListRoomsOutput{Rooms: rooms}, nil
	}
}

// CreateRoom creates a new chat room.
func CreateRoom(store *ChatStore) func(ctx context.Context, input CreateRoomInput) (Room, error) {
	return func(ctx context.Context, input CreateRoomInput) (Room, error) {
		if input.Name == "" {
			return Room{}, errors.New(errors.ErrBadRequest, "room name is required")
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		id := store.generateRoomID()
		room := Room{
			ID:        id,
			Name:      input.Name,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		store.rooms[id] = room
		store.messages[id] = nil
		return room, nil
	}
}

// ListMessages returns the message history for a room.
func ListMessages(store *ChatStore) func(ctx context.Context, input ListMessagesInput) (ListMessagesOutput, error) {
	return func(ctx context.Context, input ListMessagesInput) (ListMessagesOutput, error) {
		store.mu.RLock()
		defer store.mu.RUnlock()

		if _, ok := store.rooms[input.RoomID]; !ok {
			return ListMessagesOutput{}, errors.New(errors.ErrNotFound, "room not found: "+input.RoomID)
		}

		msgs := store.messages[input.RoomID]
		if msgs == nil {
			msgs = []Message{}
		}
		return ListMessagesOutput{Messages: msgs}, nil
	}
}

// SendMessage sends a message to a room and broadcasts it to subscribers.
func SendMessage(store *ChatStore) func(ctx context.Context, input SendMessageInput) (Message, error) {
	return func(ctx context.Context, input SendMessageInput) (Message, error) {
		if input.Content == "" {
			return Message{}, errors.New(errors.ErrBadRequest, "message content is required")
		}
		if input.Username == "" {
			return Message{}, errors.New(errors.ErrBadRequest, "username is required")
		}

		store.mu.Lock()
		if _, ok := store.rooms[input.RoomID]; !ok {
			store.mu.Unlock()
			return Message{}, errors.New(errors.ErrNotFound, "room not found: "+input.RoomID)
		}

		id := store.generateMsgID()
		msg := Message{
			ID:        id,
			RoomID:    input.RoomID,
			Username:  input.Username,
			Content:   input.Content,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}

		// Append and cap at maxMessagesPerRoom
		msgs := append(store.messages[input.RoomID], msg)
		if len(msgs) > maxMessagesPerRoom {
			msgs = msgs[len(msgs)-maxMessagesPerRoom:]
		}
		store.messages[input.RoomID] = msgs
		store.mu.Unlock()

		// Broadcast outside the lock
		store.Broadcast(input.RoomID, msg)

		return msg, nil
	}
}

// SubscribeRoom returns a channel that yields new messages in a room.
// The channel is consumed via Server-Sent Events (SSE).
func SubscribeRoom(store *ChatStore) func(ctx context.Context, input SubscribeRoomInput) (<-chan Message, error) {
	return func(ctx context.Context, input SubscribeRoomInput) (<-chan Message, error) {
		store.mu.RLock()
		_, ok := store.rooms[input.RoomID]
		store.mu.RUnlock()
		if !ok {
			return nil, errors.New(errors.ErrNotFound, "room not found: "+input.RoomID)
		}

		ch := store.Subscribe(input.RoomID)

		// Clean up when client disconnects
		go func() {
			<-ctx.Done()
			store.Unsubscribe(input.RoomID, ch)
		}()

		return ch, nil
	}
}

// HealthCheck returns the current API status.
func HealthCheck(ctx context.Context, _ struct{}) (HealthOutput, error) {
	return HealthOutput{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   gotrpc.Version,
	}, nil
}

// PlaygroundConvert converts Go type declarations to TypeScript.
func PlaygroundConvert(ctx context.Context, input PlaygroundInput) (PlaygroundOutput, error) {
	if input.Code == "" {
		return PlaygroundOutput{}, errors.New(errors.ErrBadRequest, "code is required")
	}

	ts, err := codegen.ConvertGoToTS(input.Code)
	if err != nil {
		return PlaygroundOutput{Error: err.Error()}, nil
	}

	return PlaygroundOutput{TypeScript: ts}, nil
}

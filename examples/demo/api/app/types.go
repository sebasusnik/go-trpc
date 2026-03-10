// types.go — Request/response types for each tRPC procedure.
//
// These Go structs are parsed by `gotrpc generate` to produce
// the TypeScript AppRouter type. JSON tags control the TS field names,
// and `omitempty` makes a field optional in the generated type.
package app

// Room is a chat room.
type Room struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// Message is a chat message within a room.
type Message struct {
	ID        string `json:"id"`
	RoomID    string `json:"roomId"`
	Username  string `json:"username"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

// --- Query inputs/outputs ---

type ListRoomsOutput struct {
	Rooms []Room `json:"rooms"`
}

type ListMessagesInput struct {
	RoomID string `json:"roomId"`
}

type ListMessagesOutput struct {
	Messages []Message `json:"messages"`
}

// --- Mutation inputs/outputs ---

type CreateRoomInput struct {
	Name string `json:"name"`
}

type SendMessageInput struct {
	RoomID   string `json:"roomId"`
	Username string `json:"username"`
	Content  string `json:"content"`
}

// --- Subscription inputs ---

type SubscribeRoomInput struct {
	RoomID string `json:"roomId"`
}

// --- Health ---

type HealthOutput struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// --- Playground ---

type PlaygroundInput struct {
	Code string `json:"code"`
}

type PlaygroundOutput struct {
	TypeScript string `json:"typescript"`
	Error      string `json:"error,omitempty"`
}

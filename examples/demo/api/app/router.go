package app

import (
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// NewRouter builds the full application router with all procedures registered.
func NewRouter() *gotrpc.Router {
	store := NewChatStore()

	roomRouter := gotrpc.NewRouter()
	gotrpc.Query(roomRouter, "list", ListRooms(store))
	gotrpc.Mutation(roomRouter, "create", CreateRoom(store))
	gotrpc.Query(roomRouter, "messages", ListMessages(store))

	chatRouter := gotrpc.NewRouter()
	gotrpc.Mutation(chatRouter, "send", SendMessage(store))
	gotrpc.Subscription(chatRouter, "subscribe", SubscribeRoom(store))

	healthRouter := gotrpc.NewRouter()
	gotrpc.Query(healthRouter, "check", HealthCheck)

	playgroundRouter := gotrpc.NewRouter()
	gotrpc.Query(playgroundRouter, "convert", PlaygroundConvert)

	r := gotrpc.NewRouter()
	r.Use(gotrpc.RateLimit(50))
	r.Use(gotrpc.MaxConnectionsPerIP(10))
	r.Use(gotrpc.MaxInputSize(4096))
	r.Merge("room", roomRouter)
	r.Merge("chat", chatRouter)
	r.Merge("health", healthRouter)
	r.Merge("playground", playgroundRouter)

	return r
}

package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

type Server struct {
	userToConn          map[string]map[string]*websocket.Conn
	channelToUser       map[string]map[string]struct{}
	subscribedToChannel map[string]struct{}
	subscribedToDM      map[string]struct{}
	mu                  sync.Mutex
	sessionIDCount      int64
	redisClient         *redis.Client
	ctx                 context.Context
}

func NewServer() *Server {
	ctx := context.Background()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Redis server address
	})
	err := redisClient.Ping(context.Background()).Err()
	if err != nil {
		fmt.Println("Error connecting to Redis:", err)
		os.Exit(1)
	} else {
		fmt.Println("Redis Connection established.")
	}

	return &Server{
		userToConn:          make(map[string]map[string]*websocket.Conn), // userName -> sessionID -> websocket Conn
		channelToUser:       make(map[string]map[string]struct{}),        // channelName -> userName
		subscribedToChannel: make(map[string]struct{}),                   // channelName to record channel subscriptions
		subscribedToDM:      make(map[string]struct{}),                   // userName to record DM channel subscriptions
		sessionIDCount:      1,
		redisClient:         redisClient,
		ctx:                 ctx,
	}
}

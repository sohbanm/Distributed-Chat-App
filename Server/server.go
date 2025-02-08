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
	connections    map[string]map[string]*websocket.Conn
	channels       map[string]map[string]struct{}
	subscribers    map[string]*redis.PubSub
	subscribedToDM map[string]struct{}
	mu             sync.Mutex
	sessionIDCount int64
	redisClient    *redis.Client
	ctx            context.Context
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
		connections:    make(map[string]map[string]*websocket.Conn), // userName -> sessionID -> websocket Conn
		channels:       make(map[string]map[string]struct{}),        // channelName -> userName
		subscribers:    make(map[string]*redis.PubSub),              // channelName -> redisChannel
		subscribedToDM: make(map[string]struct{}),                   // userName to record DM channel subscriptions
		sessionIDCount: 1,
		redisClient:    redisClient,
		ctx:            ctx,
	}
}

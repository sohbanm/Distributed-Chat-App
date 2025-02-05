package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type      string `json:"type"`      //'broadcast' or 'directMessage' or 'userList'
	To        string `json:"to"`        //Recipient of message
	From      string `json:"from"`      //Sender of message
	SessionID string `json:"sessionID"` //SessionID of Sender
	Message   string `json:"message"`   //message content
}

type Server struct {
	connections    map[string]map[string]*websocket.Conn
	channels       map[string]map[string]struct{}
	subscribers    map[string]*redis.PubSub
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
		sessionIDCount: 1,
		redisClient:    redisClient,
		ctx:            ctx,
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) generateSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate a unique session ID (incremental)
	sessionID := fmt.Sprintf("%d", s.sessionIDCount)
	s.sessionIDCount++
	return sessionID
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Upgrade error:", err)
		return
	}

	fmt.Println("New WebSocket connection established to the server:", conn.RemoteAddr())
	defer conn.Close()

	username := r.URL.Query().Get("username")
	if username == "" {
		username = fmt.Sprintf("user-%d", len(s.connections)+1)
	}

	//Generate a new Session ID
	sessionID := s.generateSessionID()

	// Add to connections
	s.mu.Lock()
	if _, exists := s.connections[username]; !exists {
		s.connections[username] = make(map[string]*websocket.Conn)
		s.addUserToRedis(username)
		s.subscribeToDirectMessage(username)
	}
	s.connections[username][sessionID] = conn
	s.mu.Unlock()

	// Notify the client of its session ID
	initialMessage := Message{
		Type:    "sessionID",
		Message: sessionID,
	}
	initialMessageBytes, _ := json.Marshal(initialMessage)
	if err := conn.WriteMessage(websocket.TextMessage, initialMessageBytes); err != nil {
		fmt.Println("Error sending session ID to client:", err)
		return
	}

	// Subscribe to the server channel
	serverSubscriber := s.redisClient.Subscribe(s.ctx, "server")
	go s.subscribeToListUpdates(serverSubscriber)

	// Broadcast the user & channel list to all clients
	s.broadcastUserList()
	s.broadcastChannelList()

	defer func() {
		s.mu.Lock()
		delete(s.connections[username], sessionID)
		if len(s.connections[username]) == 0 {
			s.removeUserFromRedis(username)
			delete(s.connections, username)
		}
		s.mu.Unlock()
		fmt.Printf("User %s (session %s) disconnected\n", username, sessionID)

		//Write to Redis Channel "server"
		s.broadcastUserList()
	}()

	// Read incoming messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Read error:", err)
			break
		}
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			fmt.Println("Invalid message format:", err)
			continue
		}

		if msg.Type == "broadcast" {
			s.joinChannel(msg.From, msg.To)
			s.broadcast(msg.From, msg.To, []byte(msg.Message))
		} else if msg.Type == "directMessage" && msg.To != "" && msg.From != "" {
			if err := s.redisClient.Publish(s.ctx, "user:"+msg.To, message).Err(); err != nil {
				fmt.Printf("Error publishing message to user %s: %v\n", msg.To, err)
			}
		} else if msg.Type == "createChannel" && msg.From != "" {
			s.createChannel(msg.From, []byte(msg.Message))
		}

		fmt.Printf("%s: New Message from %s (session %s), Message: %s\n", conn.RemoteAddr(), username, sessionID, message)
	}
}

// ----------ISOLATED METHODS----------
func (s *Server) createChannel(from string, messageText []byte) {
	var channelName = string(messageText)
	s.mu.Lock()
	if _, exists := s.channels[channelName]; exists {
		s.mu.Unlock()
	} else {
		s.channels[channelName] = make(map[string]struct{})
		s.channels[channelName][from] = struct{}{}
		s.mu.Unlock()
		s.addChannelToRedis(channelName)
		s.broadcastChannelList()
	}

	s.subscribeToGroup(channelName)
}

func (s *Server) marshalMessage(message Message) ([]byte, error) {
	messageJSON, err := json.Marshal(message)
	if err != nil {
		fmt.Printf("Error marshaling message of type %s: %v\n", message.Type, err)
		return nil, err
	}
	return messageJSON, nil
}

// ---- REDIS ----

// REDIS LIST UPDATE AND GETTERS
func (s *Server) addUserToRedis(username string) {
	if err := s.redisClient.SAdd(s.ctx, "userList", username).Err(); err != nil {
		fmt.Printf("Error adding user %s to Redis: %v\n", username, err)
	}
}

func (s *Server) removeUserFromRedis(username string) {
	if err := s.redisClient.SRem(s.ctx, "userList", username).Err(); err != nil {
		fmt.Printf("Error removing user %s from Redis: %v\n", username, err)
	}
}

func (s *Server) getUserListFromRedis() ([]string, error) {
	userList, err := s.redisClient.SMembers(s.ctx, "userList").Result()
	if err != nil {
		return nil, fmt.Errorf("error retrieving user list from Redis: %v", err)
	}
	return userList, nil
}

func (s *Server) addChannelToRedis(channelName string) {
	if err := s.redisClient.SAdd(s.ctx, "channelList", channelName).Err(); err != nil {
		fmt.Printf("Error adding channel %s to Redis: %v\n", channelName, err)
	}
}

func (s *Server) removeChannelFromRedis(channelName string) {
	if err := s.redisClient.SRem(s.ctx, "channelList", channelName).Err(); err != nil {
		fmt.Printf("Error removing channel %s from Redis: %v\n", channelName, err)
	}
}

func (s *Server) getChannelListFromRedis() ([]string, error) {
	channelList, err := s.redisClient.SMembers(s.ctx, "channelList").Result()
	if err != nil {
		return nil, fmt.Errorf("error retrieving channel list from Redis: %v", err)
	}
	return channelList, nil
}

// REDIS LISTS HANDLER
func (s *Server) subscribeToListUpdates(subscriber *redis.PubSub) {
	for {
		msg, err := subscriber.ReceiveMessage(s.ctx)

		if err != nil {
			fmt.Printf("Error receiving SERVER update: %v\n", err)
			return
		}

		var message Message
		if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
			fmt.Printf("Error unmarshaling message for SERVER: %v\n", err)
			continue
		}

		if message.Type == "userList" {
			var userList []string
			if err := json.Unmarshal([]byte(message.Message), &userList); err == nil {
				s.broadcastMessage([]byte(msg.Payload), "")
			} else {
				fmt.Printf("Error unmarshaling user list: %v\n", err)
			}
		} else if message.Type == "channelList" {
			var channelList []string
			if err := json.Unmarshal([]byte(message.Message), &channelList); err == nil {
				s.broadcastMessage([]byte(msg.Payload), "")
			} else {
				fmt.Printf("Error unmarshaling channel list: %v\n", err)
			}
		} else {
			fmt.Printf("Invalid message type for SERVER: %s\n", message.Type)
		}
	}
}

// REDIS SUBSCRIPTION DIRECT MESSAGING
func (s *Server) subscribeToDirectMessage(user string) {
	subscriber := s.redisClient.Subscribe(s.ctx, "user:"+user)
	go s.handleMessages(user, subscriber)
}

// REDIS SUBSCRIPTION GROUP MESSAGING
func (s *Server) joinChannel(username string, channelName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add the user to the channel in memory
	if _, exists := s.channels[channelName]; !exists {
		s.channels[channelName] = make(map[string]struct{})
		s.addChannelToRedis(channelName)
	}

	s.channels[channelName][username] = struct{}{}
	s.subscribeToGroup(channelName)
}

func (s *Server) subscribeToGroup(channelName string) {
	if _, exists := s.subscribers[channelName]; exists {
		fmt.Printf("Already connected to the channel %s\n", channelName)
		return
	}
	subscriber := s.redisClient.Subscribe(s.ctx, "channel:"+channelName)
	s.subscribers[channelName] = subscriber

	// Start a goroutine to handle incoming messages for this channel
	go s.handleMessages(channelName, subscriber)
}

// REDIS MESSAGE HANDLER
func (s *Server) handleMessages(channelName string, subscriber *redis.PubSub) {
	for {
		msg, err := subscriber.ReceiveMessage(s.ctx)

		if err != nil {
			fmt.Printf("Error receiving message for channel %s: %v\n", channelName, err)
			return
		}

		var message Message
		if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
			fmt.Printf("Error unmarshaling message for channel %s: %v\n", channelName, err)
			continue
		}
		if message.Type == "broadcast" {
			s.broadcastMessage([]byte(msg.Payload), channelName)
		} else if message.Type == "directMessage" {
			s.directMessage(message.To, message.From, message.SessionID, []byte(message.Message))
		} else {
			fmt.Printf("Invalid message type for channel %s: %s\n", channelName, message.Type)
		}
	}
}

// ----------BROADCASTING----------
func (s *Server) broadcast(from string, to string, messageText []byte) {
	message, err := s.marshalMessage(Message{
		Type:    "broadcast",
		From:    from,
		To:      to,
		Message: string(messageText),
	})
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.redisClient.Publish(s.ctx, "channel:"+to, message).Err(); err != nil {
		fmt.Printf("Error publishing message to channel %s: %v\n", to, err)
	}
}

func (s *Server) broadcastUserList() {
	userList, err := s.getUserListFromRedis()
	if err != nil {
		fmt.Println("Error retrieving user list from Redis:", err)
		return
	}

	userListJSON, err := json.Marshal(userList)
	if err != nil {
		fmt.Println("Error marshaling user list:", err)
		return
	}

	message, err := s.marshalMessage(Message{
		Type:    "userList",
		Message: string(userListJSON),
	})
	if err != nil {
		return
	}

	if err := s.redisClient.Publish(s.ctx, "server", message).Err(); err != nil {
		fmt.Printf("Error updating User List to SERVER: %v\n", err)
	}
}

func (s *Server) broadcastChannelList() {
	channelList, err := s.getChannelListFromRedis()
	if err != nil {
		fmt.Println("Error retrieving channel list from Redis:", err)
		return
	}

	userListJSON, err := json.Marshal(channelList)
	if err != nil {
		fmt.Println("Error marshaling channel list:", err)
		return
	}

	message, err := s.marshalMessage(Message{
		Type:    "channelList",
		Message: string(userListJSON),
	})
	if err != nil {
		return
	}

	if err := s.redisClient.Publish(s.ctx, "server", message).Err(); err != nil {
		fmt.Printf("Error updating Channel List to SERVER: %v\n", err)
	}
}

func (s *Server) broadcastMessage(message []byte, channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channel == "" {
		//send list of usernames/channels to all connected clients
		for username, sessions := range s.connections {
			for sessionID, conn := range sessions {
				s.sendMessage(conn, message, username, sessions, sessionID, channel)
			}
		}
	} else {
		// Send message to all connected clients of the channel
		if users, exists := s.channels[channel]; exists {
			for username := range users {
				sessions := s.connections[username]
				for sessionID, conn := range sessions {
					s.sendMessage(conn, message, username, sessions, sessionID, "")
				}
			}
		} else {
			fmt.Printf("Channel %s was not found to be broadcasted\n", channel)
		}
	}
}

// ----------DIRECT MESSAGING----------
func (s *Server) directMessage(to string, from string, sessionID string, messageText []byte) {
	s.mu.Lock()
	sessions, exists := s.connections[to]
	s.mu.Unlock()

	if !exists {
		fmt.Printf("User %s not found for direct message\n", to)
		return
	}

	message, err := s.marshalMessage(Message{
		Type:    "directMessage",
		Message: string(messageText),
		To:      to,
		From:    from,
	})
	if err != nil {
		return
	}

	for sessionID, conn := range sessions {
		s.mu.Lock()
		s.sendMessage(conn, message, to, sessions, sessionID, "")
		s.mu.Unlock()
	}

	s.updateOtherSessions(to, from, sessionID, messageText)
}

func (s *Server) updateOtherSessions(to string, from string, currentSessionID string, messageText []byte) {
	s.mu.Lock()
	sessions := s.connections[from]
	s.mu.Unlock()

	message, err := s.marshalMessage(Message{
		Type:    "sessionUpdate",
		Message: string(messageText),
		To:      to,
		From:    from,
	})
	if err != nil {
		return
	}

	for sessionID, conn := range sessions {
		if sessionID == currentSessionID {
			continue
		}
		s.mu.Lock()
		s.sendMessage(conn, message, to, sessions, sessionID, "")
		s.mu.Unlock()
	}

}

// ----------SENDING MESSAGE----------
func (s *Server) sendMessage(conn *websocket.Conn, message []byte, username string, sessions map[string]*websocket.Conn, sessionID string, channel string) {
	if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
		fmt.Printf("Write error for user %s (session %s): %v\n", username, sessionID, err)
		conn.Close()
		delete(sessions, sessionID)
		if len(sessions) == 0 {
			s.removeUserFromRedis(username)
			delete(s.connections, username)
			if channel != "" {
				if _, exists := s.channels[channel]; exists {
					delete(s.channels[channel], username) // Remove user from the channel map
				}
				if len(s.channels[channel]) == 0 {
					delete(s.channels, channel)
				}
			}
		}
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	s.broadcastUserList()
	s.broadcastChannelList()

	defer func() {
		s.mu.Lock()
		delete(s.connections[username], sessionID)
		if len(s.connections[username]) == 0 {
			delete(s.connections, username)
		}
		s.mu.Unlock()
		fmt.Printf("User %s (session %s) disconnected\n", username, sessionID)

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
			s.directMessage(msg.To, msg.From, msg.SessionID, []byte(msg.Message))
		} else if msg.Type == "createChannel" && msg.From != "" {
			s.createChannel(msg.From, []byte(msg.Message))
		}

		fmt.Printf("%s: New Message from %s (session %s), Message: %s\n", conn.RemoteAddr(), username, sessionID, message)
	}
}

// ----------ISOLATED METHODS----------
func (s *Server) createChannel(from string, messageText []byte) {
	var message = string(messageText)
	s.mu.Lock()
	s.channels[message] = make(map[string]struct{})
	s.channels[message][from] = struct{}{}
	s.mu.Unlock()

	s.broadcastChannelList()
	s.subscribeToGroup(message)
}

func (s *Server) marshalMessage(message Message) ([]byte, error) {
	messageJSON, err := json.Marshal(message)
	if err != nil {
		fmt.Printf("Error marshaling message of type %s: %v\n", message.Type, err)
		return nil, err
	}
	return messageJSON, nil
}

// ----------LIST GENERATION----------
func (s *Server) createUserList() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	userList := make([]string, 0, len(s.connections))
	for username := range s.connections {
		userList = append(userList, username)
	}
	return userList
}

func (s *Server) createChannelList() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelList := make([]string, 0, len(s.channels))
	for channelName := range s.channels {
		channelList = append(channelList, channelName)
	}
	return channelList
}

// REDIS SUBSCRIPTION

func (s *Server) joinChannel(username string, channelName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add the user to the channel in memory
	if _, exists := s.channels[channelName]; !exists {
		s.channels[channelName] = make(map[string]struct{})
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
	go s.handleGroupMessages(channelName, subscriber)
}

func (s *Server) handleGroupMessages(channelName string, subscriber *redis.PubSub) {
	for {
		msg, err := subscriber.ReceiveMessage(s.ctx)

		if err != nil {
			fmt.Printf("Error receiving message for channel %s: %v\n", channelName, err)
			return
		}

		// Broadcast the message to WebSocket clients in this channel
		var message Message
		if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
			fmt.Printf("Error unmarshaling message for channel %s: %v\n", channelName, err)
			continue
		}
		s.broadcastMessage([]byte(msg.Payload), channelName)
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
		return
	}
}

func (s *Server) broadcastUserList() {
	userList := s.createUserList()

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

	s.broadcastMessage(message, "")
}

func (s *Server) broadcastChannelList() {
	channelList := s.createChannelList()

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

	s.broadcastMessage(message, "")
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
	sessions, _ := s.connections[from]
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

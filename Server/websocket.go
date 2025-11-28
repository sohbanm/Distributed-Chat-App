package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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
		username = fmt.Sprintf("user-%d", len(s.userToConn)+1)
	}

	// Generate a new Session ID
	sessionID := uuid.New().String()

	// Add to connections
	s.mu.Lock()
	if _, exists := s.userToConn[username]; !exists {
		s.userToConn[username] = make(map[string]*websocket.Conn)
		s.addUserToRedis(username)
		s.subscribeToDirectMessage(username)
	}
	s.userToConn[username][sessionID] = conn
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
		delete(s.userToConn[username], sessionID)
		if len(s.userToConn[username]) == 0 {
			delete(s.userToConn, username)
		}
		s.mu.Unlock()
		fmt.Printf("User %s (session %s) disconnected\n", username, sessionID)

		// Write to Redis Channel "server"
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

		if msg.Type == "ping" {
			fmt.Printf("Ping received from %s\n", username)
			pongMsg := Message{Type: "pong", Message: "pong"}
			pongBytes, _ := json.Marshal(pongMsg)
			conn.WriteMessage(websocket.TextMessage, pongBytes)
			continue
		}

		if msg.Type == "broadcast" {
			s.joinChannel(msg.From, msg.To)
			s.broadcast(msg.From, msg.To, []byte(msg.Message))
		} else if msg.Type == "directMessage" && msg.To != "" && msg.From != "" {
			if err := s.redisClient.Publish(s.ctx, "user:"+msg.To, message).Err(); err != nil {
				fmt.Printf("Error publishing message to user %s: %v\n", msg.To, err)
			}
			updateMsg := msg
			updateMsg.Type = "sessionUpdate"
			updateBytes, _ := json.Marshal(updateMsg)

			if err := s.redisClient.Publish(s.ctx, "user:"+msg.From, updateBytes).Err(); err != nil {
				fmt.Printf("Error publishing session update to user %s: %v\n", msg.From, err)
			}
		} else if msg.Type == "createChannel" && msg.From != "" {
			s.createChannel(msg.From, []byte(msg.Message))
		}

		fmt.Printf("%s: New Message from %s (session %s), Message: %s\n", conn.RemoteAddr(), username, sessionID, message)
	}
}

func (s *Server) createChannel(from string, messageText []byte) {
	var channelName = string(messageText)

	s.addChannelToRedis(channelName)
	s.broadcastChannelList()

	joinMsg := Message{
		Type:    "channelJoin",
		From:    from,
		To:      channelName, // We reuse 'To' for the channel name here
		Message: channelName,
	}
	joinMsgBytes, _ := json.Marshal(joinMsg)

	if err := s.redisClient.Publish(s.ctx, "user:"+from, joinMsgBytes).Err(); err != nil {
		fmt.Printf("Error publishing channel join for user %s: %v\n", from, err)
	}
}

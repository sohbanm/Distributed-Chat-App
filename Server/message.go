package main

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type      string `json:"type"`      //'broadcast' or 'directMessage' or 'userList' or 'channelList' or 'sessionUpdate'
	To        string `json:"to"`        // Recipient of message
	From      string `json:"from"`      // Sender of message
	SessionID string `json:"sessionID"` // SessionID of Sender
	Message   string `json:"message"`   // Message content
}

func (s *Server) marshalMessage(message Message) ([]byte, error) {
	messageJSON, err := json.Marshal(message)
	if err != nil {
		fmt.Printf("Error marshaling message of type %s: %v\n", message.Type, err)
		return nil, err
	}
	return messageJSON, nil
}

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
		// Send list of usernames/channels to all connected clients
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
		fmt.Println("Error marshaling direct message:", err)
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

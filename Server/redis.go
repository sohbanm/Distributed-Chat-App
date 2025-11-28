package main

import (
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
)

// func (s *Server) removeUserFromRedis(username string) {
// 	if err := s.redisClient.SRem(s.ctx, "userList", username).Err(); err != nil {
// 		fmt.Printf("Error removing user %s from Redis: %v\n", username, err)
// 	}
// }

func (s *Server) addUserToRedis(username string) {
	if err := s.redisClient.SAdd(s.ctx, "userList", username).Err(); err != nil {
		fmt.Printf("Error adding user %s to Redis: %v\n", username, err)
	}
}

func (s *Server) getUserListFromRedis() ([]string, error) {
	userList, err := s.redisClient.SMembers(s.ctx, "userList").Result()
	if err != nil {
		return nil, fmt.Errorf("error retrieving user list from Redis: %v", err)
	}
	return userList, nil
}

// func (s *Server) removeChannelFromRedis(channelName string) {
// 	if err := s.redisClient.SRem(s.ctx, "channelList", channelName).Err(); err != nil {
// 		fmt.Printf("Error removing channel %s from Redis: %v\n", channelName, err)
// 	}
// }

func (s *Server) addChannelToRedis(channelName string) {
	if err := s.redisClient.SAdd(s.ctx, "channelList", channelName).Err(); err != nil {
		fmt.Printf("Error adding channel %s to Redis: %v\n", channelName, err)
	}
}

func (s *Server) getChannelListFromRedis() ([]string, error) {
	channelList, err := s.redisClient.SMembers(s.ctx, "channelList").Result()
	if err != nil {
		return nil, fmt.Errorf("error retrieving channel list from Redis: %v", err)
	}
	return channelList, nil
}

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

func (s *Server) subscribeToDirectMessage(user string) {
	if _, exists := s.subscribedToDM[user]; exists {
		fmt.Printf("Already connected to direct messages for %s\n", user)
		return
	}
	subscriber := s.redisClient.Subscribe(s.ctx, "user:"+user)
	s.subscribedToDM[user] = struct{}{}

	go s.handleMessages(user, subscriber)
}

func (s *Server) joinChannel(username string, channelName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add the user to the channel in memory
	if _, exists := s.channelToUser[channelName]; !exists {
		s.channelToUser[channelName] = make(map[string]struct{})
	}

	s.channelToUser[channelName][username] = struct{}{}
	s.subscribeToGroup(channelName)
}

func (s *Server) subscribeToGroup(channelName string) {
	if _, exists := s.subscribedToChannel[channelName]; exists {
		fmt.Printf("Already connected to the channel %s\n", channelName)
		return
	}
	subscriber := s.redisClient.Subscribe(s.ctx, "channel:"+channelName)
	s.subscribedToChannel[channelName] = struct{}{}

	// Start a goroutine to handle incoming messages for this channel
	go s.handleMessages(channelName, subscriber)
}

func (s *Server) handleMessages(channelName string, subscriber *redis.PubSub) {
	fmt.Println("Subscribed and listening to channel", channelName)
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
		} else {
			fmt.Printf("Received message for channel %s: %s\n", channelName, message.Message)
		}

		if message.Type == "broadcast" {
			s.broadcastMessage([]byte(msg.Payload), channelName)
		} else if message.Type == "directMessage" {
			s.directMessage(message.To, message.From, message.SessionID, []byte(message.Message))
		} else if message.Type == "sessionUpdate" {
			s.updateOtherSessions(message.To, message.From, message.SessionID, []byte(message.Message))
		} else if message.Type == "channelJoin" {
			s.joinChannel(message.From, message.To)
		} else {
			fmt.Printf("Invalid message type for channel %s: %s\n", channelName, message.Type)
		}
	}
}

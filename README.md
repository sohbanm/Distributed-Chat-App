# Distributed-Chat-App

## Overview

This project is the backend for a distributed chat application. It facilitates real-time communication between users using WebSocket connections and Redis for managing message broadcasting and channel subscriptions. The backend is implemented in Go and supports features like direct messaging, broadcast messaging, and channel-based communication.

## Features

- **WebSocket Connections**: Supports real-time, bidirectional communication.
- **Redis Integration**: Utilizes Redis for message broadcasting and subscription management.
- **User Management**: Tracks active users and their sessions, providing real-time updates.
- **Channel Management**: Allows users to create, join, and communicate in channels.
- **Direct Messaging**: Enables private messages between users.
- **Session Tracking**: Generates unique session IDs for each user connection.

## Key Endpoints

- **WebSocket Connection**:  
  Establish a WebSocket connection by connecting to the server with a username parameter.

  Example:  
  `ws://localhost:PORT/ws?username=<your-username>`

## How It Works

1. **Connection Handling**: 
   Users connect via WebSocket and are assigned a unique session ID. Connections are tracked per user and session.

2. **Message Types**:
   - **Broadcast**: Messages sent to a specific channel.
   - **Direct Message**: Messages sent privately to a specific user.
   - **Channel Management**: Users can create or join channels.

3. **Redis Pub/Sub**:
   - Used for distributing messages across instances in a scalable setup.
   - Handles real-time updates to channel subscribers.

4. **User and Channel Lists**:
   - Maintains updated lists of active users and channels, broadcasting changes as necessary.

## Running the Backend

1. **Dependencies**:
   - Go 1.19 or later
   - Redis Server running on `localhost:6379`

2. **Starting the Server**:
   ```bash
   go run main.go
   ```

3. **Connecting Clients**:
   Use a WebSocket client to connect to the server and start sending/receiving messages.

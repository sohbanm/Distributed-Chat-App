import React, { useEffect, useState, useRef } from "react";

interface Message {
  type: "broadcast" | "directMessage" | "createChannel" | "userList" | "channelList" | "sessionID" | "sessionUpdate";
  to?: string;
  from?: string;
  sessionID?: string;
  message: string;
}

function App() {
  const [ws, setWs] = useState<WebSocket | null>(null);
  const [username, setUsername] = useState<string>("");
  const [sessionID, setSessionID] = useState<string | null>(null);
  const [connectedUsers, setConnectedUsers] = useState<string[]>([]);
  const [connectedChannels, setConnectedChannels] = useState<string[]>([]);
  const [broadcastMessage, setBroadcastMessage] = useState<string>("");
  const [channelName, setChannelName] = useState<string>(""); //new
  const [directMessage, setDirectMessage] = useState<string>("");
  const [selectedUser, setSelectedUser] = useState<string>("");
  const [selectedChannel, setSelectedChannel] = useState<string>("");
  const [messages, setMessages] = useState<{ from: string; to: string; message: string; type: string; }[]>([]);

  const [isLoggedIn, setIsLoggedIn] = useState(false);

  useEffect(() => {
    if (isLoggedIn && username) {
      const socket = new WebSocket(`ws://localhost:3001/chat?username=${username}`);

      socket.onopen = () => {
        console.log("WebSocket connection established.");
      };

      socket.onmessage = (event) => {
        const msg: Message = JSON.parse(event.data);
        console.log(msg);
      
        if (msg.type === "broadcast" || msg.type === "directMessage" || msg.type === "sessionUpdate") {
          setMessages((prevMessages) => [
            ...prevMessages,
            {
              from: msg.from || "Server",
              to: msg.to || (msg.type === "broadcast" ? "CHANNEL" : "Client"),
              message: msg.message,
              type: msg.type,
            }
          ]);
        }
      
        if (msg.type === "userList") {
          const updatedUsers = JSON.parse(msg.message) as string[];
          setConnectedUsers(updatedUsers.filter((user) => user !== username));
        } else if (msg.type === "channelList") {
          const updatedChannels = JSON.parse(msg.message) as string[];
          setConnectedChannels(updatedChannels);
        } else if (msg.type === "sessionID") {
          setSessionID(msg.message);
        }
      };
      
      
      socket.onerror = (error) => {
        console.error("WebSocket error:", error);
      };

      socket.onclose = () => {
        console.log("WebSocket connection closed.");
      };

      setWs(socket);

      return () => {
        socket.close();
      };
    }
  }, [isLoggedIn, username]);

  const createNewChannel = () => {
    if (ws && channelName.trim().length > 0) {
      const msg: Message = {
        type: "createChannel",
        message: channelName,
        from: username,
      };
      ws.send(JSON.stringify(msg));
      setChannelName("");
    } else {
      alert("Please enter a channel name.");
    }
  };

  const sendBroadcastMessage = () => {
    if (ws && broadcastMessage.length > 0 && selectedChannel.trim().length > 0) {
      const msg: Message = {
        type: "broadcast",
        message: broadcastMessage,
        from: username,
        to: selectedChannel,
      };
      ws.send(JSON.stringify(msg));
      setBroadcastMessage("");
      setSelectedChannel("");
    } else {
      alert("Please select a broadcast channel and enter a message.");
    }
  };

  const sendDirectMessage = () => {
    if (ws && selectedUser) {
      const msg: Message = { type: "directMessage", to: selectedUser, from: username, message: directMessage, ...(sessionID && { sessionID }) };      
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          from: msg.from || "Server",
          to: msg.to || (msg.type === "broadcast" ? "CHANNEL" : "Client"),
          message: msg.message,
          type: msg.type,
        }
      ]);
      ws.send(JSON.stringify(msg));
      setDirectMessage("");
    }
  };

  const handleLogin = () => {
    if (username.trim()) {
      setIsLoggedIn(true);
    } else {
      alert("Please enter a username.");
    }
  };

  return (
    <div>
      {!isLoggedIn ? (
        <div>
          <h2>Enter Your Username</h2>
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Username"
          />
          <button onClick={handleLogin}>Join Chat</button>
        </div>
      ) : (
        <div>
          <h1>WebSocket Chat for "{username}" session "{sessionID}"</h1>

          <div>
            <h2>Broadcast Message</h2>

            <div>
              <input
                type="text"
                value={channelName}
                onChange={(e) => setChannelName(e.target.value)}
                placeholder="Create a Channel to broadcast to..."
              />
              <button onClick={createNewChannel}>Create Channel</button>
            </div>

            <div>
              <select
                value={selectedChannel}
                onChange={(e) => setSelectedChannel(e.target.value)}
              >
                <option value="" disabled>
                  Select a channel
                </option>
                {connectedChannels.map((user) => (
                  <option key={user} value={user}>
                    {user}
                  </option>
                ))}
              </select>

              <input
                type="text"
                value={broadcastMessage}
                onChange={(e) => setBroadcastMessage(e.target.value)}
                placeholder="Type a broadcast message..."
              />
              <button onClick={sendBroadcastMessage}>Send Broadcast</button>
            </div>
          </div>

          <div>
            <h2>Direct Message</h2>

            <select
              value={selectedUser}
              onChange={(e) => setSelectedUser(e.target.value)}
            >
              <option value="" disabled>
                Select a user
              </option>
              {connectedUsers.map((user) => (
                <option key={user} value={user}>
                  {user}
                </option>
              ))}
            </select>

            <input
              type="text"
              value={directMessage}
              onChange={(e) => setDirectMessage(e.target.value)}
              placeholder="Type a direct message..."
            />

            <button onClick={sendDirectMessage}>Send Direct Message</button>
          </div>
          <div>
            <div>
              <h2>Messages</h2>
              <div
                style={{
                  maxHeight: "600px",
                  overflowY: "auto",
                  border: "1px solid #ddd",
                  borderRadius: "8px",
                  padding: "10px",
                  backgroundColor: "#f9f9f9",
                  maxWidth: "75%",
                  margin: "0 auto",
                }}
              >
                {messages.map((msg, index) => {
                  const isSentByUser = msg.from !== username;

                  return (
                    <div
                      key={index}
                      style={{
                        display: "flex",
                        justifyContent: isSentByUser ? "flex-end" : "flex-start",
                        marginBottom: "8px",
                      }}
                    >
                      <div
                        style={{
                          maxWidth: "60%",
                          minWidth: "10%",
                          padding: "8px",
                          borderRadius: "6px",
                          backgroundColor: isSentByUser ? "#d6eaff" : "#d4f8d4", // Light blue for sent, light green for received
                          boxShadow: "0 1px 3px rgba(0,0,0,0.1)",
                          textAlign: "left",
                        }}
                      >
                        {/* From and To section */}
                        <div
                          style={{
                            display: "flex",
                            justifyContent: "space-between",
                            marginBottom: "4px",
                          }}
                        >
                          <strong style={{ color: "#000", fontWeight: "normal" }}>
                            From <span style={{ color: "#007bff", fontWeight: "bold" }}>{msg.from}</span>
                          </strong>
                          {msg.type === "directMessage" || msg.type === "sessionUpdate" ? (
                            <span style={{ color: "#000"}}>
                              To <span style={{ color: "#28a745", fontWeight: "bold" }}>{msg.to}</span>
                            </span>
                          ) : (
                            <span style={{ color: "#000"}}>
                              in channel <span style={{ color: "#28a745", fontWeight: "bold" }}>{msg.to}</span>
                            </span>
                          )}
                        </div>

                        {/* Message Content */}
                        <p style={{ margin: "4px 0", color: "#333" }}>{msg.message}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default App
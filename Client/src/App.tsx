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
  const [messages, setMessages] = useState<string[]>([]);
  const [isLoggedIn, setIsLoggedIn] = useState(false);

  useEffect(() => {
    if (isLoggedIn && username) {
      const socket = new WebSocket(`ws://localhost:3000/chat?username=${username}`);

      socket.onopen = () => {
        console.log("WebSocket connection established.");
      };

      socket.onmessage = (event) => {
        const msg: Message = JSON.parse(event.data);
        console.log(msg)
        
        if (msg.type === "broadcast") {
          setMessages((prevMessages) => [...prevMessages, `In channel "${msg.to || "CHANNEL"}" "${msg.from || "Server"}" said: ${msg.message}`]);
        } else if (msg.type === "directMessage") {
          setMessages((prevMessages) => [...prevMessages, `${msg.from || "Server"} said to ${msg.to || "Client"}: ${msg.message}`]);
        } else if (msg.type === "userList") {    
          const updatedUsers = JSON.parse(msg.message) as string[];
          setConnectedUsers(updatedUsers.filter((user) => user !== username));
        } else if (msg.type === "channelList") {
          const updatedChannels = JSON.parse(msg.message) as string[];
          setConnectedChannels(updatedChannels);
        } else if (msg.type === "sessionID") {
          const newSessionID = msg.message;
          setSessionID(newSessionID);
        } else if (msg.type === "sessionUpdate") {
          setMessages((prevMessages) => [...prevMessages, `${msg.from || "Server"} said to ${msg.to || "Client"}: ${msg.message}`]);
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
      setMessages((prevMessages) => [...prevMessages, `${msg.from || "Server"} said to ${msg.to || "Client"}: ${msg.message}`]);
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
{/*  */}
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
            <h2>Messages</h2>
            <ul>
              {messages.map((msg, index) => (
                <li key={index}>{msg}</li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </div>
  );
};

export default App
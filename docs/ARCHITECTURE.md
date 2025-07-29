# Atlas Backend Architecture

## Project Overview

This is a Go application that serves as the back-end for the Atlas Multi-Game. It uses WebSockets for real-time communication and Protocol Buffers (protobuf) for data serialization.

## Architecture

The project is structured as a standard Go application. The main application logic is in `main.go`, which handles HTTP requests for creating game rooms. The core game logic and WebSocket handling are located in the `internal` directory.

-   **`main.go`**: The entry point of the application. It exposes an HTTP endpoint (`/create`) to create new game rooms. For each new room, it launches a new goroutine to handle WebSocket connections for that room.
-   **`internal/game_room`**: This package defines the `GameRoom` struct, which manages the state of a single game room, including player data and connections.
-   **`internal/web_socket`**: This package is responsible for handling WebSocket connections. It uses the `gorilla/websocket` library to upgrade HTTP connections to WebSockets and manages the lifecycle of the connection.
-   **`internal/protobufs`**: This directory contains the Protocol Buffers definitions (`.proto` files) and the generated Go code.

## Real-time Communication

The application uses WebSockets for real-time, bidirectional communication between the server and the clients.

### Connection Lifecycle

1.  The client sends an HTTP request to the `/create` endpoint.
2.  The server creates a new game room with a unique `RoomID` and sends it back in the HTTP response.
3.  The server starts a new WebSocket handler for the created room, listening for connections on the `/{RoomID}` endpoint.
4.  The client uses the `RoomID` to establish a WebSocket connection to `ws://<server-address>/{RoomID}`.
5.  Upon a successful connection, the server sends a `SEND_ON_CONNECT_ACK` message to the client, containing the `RoomID` and a unique `PlayerID` for the client.

### Protobuf Schema

The application uses Protocol Buffers to define the structure of the messages sent over the WebSocket connection.

#### Client-to-Server Messages

-   **`REQUEST_GAME_STATE`**: Requests the full game state from the server.
-   **`SEND_GAME_UPDATE`**: Sends a player's game action to the server.
    -   Payload: `GameUpdatePayload` (`level`, `type`)
-   **`SEND_CHAT_MESSAGE`**: Sends a chat message to the server.
    -   Payload: `ChatMessagePayload` (`sender_id`, `content`)

#### Server-to-Client Messages

-   **`RESPOND_GAME_STATE`**: The server's response to a `REQUEST_GAME_STATE` message.
    -   Payload: `GameStatePayload` (`level`, `is_started`, `is_over`, `currentLetter`)
-   **`BROADCAST_GAME_UPDATE`**: A game update broadcasted to all players in a room.
    -   Payload: `GameUpdatePayload` (`level`, `type`)
-   **`BROADCAST_CHAT_MESSAGE`**: A chat message broadcasted to all players in a room.
    -   Payload: `ChatMessagePayload` (`sender_id`, `content`)
-   **`SEND_ON_CONNECT_ACK`**: An acknowledgment message sent to a client upon successful connection.
    -   Payload: `OnConnectAckPayload` (`roomId`, `playerId`)

#### Data Structures

-   **`PlayerData`**: Represents a player's data (`player_id`, `name`, `hearts`, `score`).

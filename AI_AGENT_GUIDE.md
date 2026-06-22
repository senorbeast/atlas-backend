# Atlas Backend AI Agent Guide

This guide documents the current backend architecture, structure, style, and safe extension points for AI agents.

## Overview

`atlas-backend` is a small Go service for multiplayer Atlas rooms. It uses:

- Standard `net/http` for HTTP routing.
- `github.com/gorilla/websocket` for WebSocket upgrades and binary frames.
- `google.golang.org/protobuf` for message serialization.
- In-memory maps protected by mutexes for room and player connection state.

Current behavior:

- `GET /create` creates a new game room.
- The room gets a dynamic WebSocket path: `/{roomId}`.
- On connection, the server generates a player id, stores the connection, and sends a protobuf connect ACK.
- Binary protobuf chat messages are accepted and broadcast to all players in the room.
- Game state and game update handlers exist but are placeholders.

## Runtime Flow

1. `main.go` registers `/create` with `createGameRoomHandler`.
2. `createGameRoomHandler` generates a room id, creates a `game_room.GameRoom`, stores it in the package-level `gameRooms` map, then calls `web_socket.HandleWebSocketConnections(gameRoom)`.
3. `HandleWebSocketConnections` registers an HTTP handler for `/{roomId}`.
4. When a client connects, Gorilla upgrades the request to WebSocket.
5. The server generates a player id, stores `PlayerConnection` in `GameRoom.PlayerData`, and sends `SEND_ON_CONNECT_ACK`.
6. `HandleAllMessage` reads binary frames in a loop, unmarshals `ClientToServerMessage`, switches on `message_type`, and delegates to handler functions.
7. Response helpers marshal `ServerToClientMessage` and send or broadcast binary frames.

## Directory Structure

```text
.
|-- main.go
|-- go.mod
|-- internal/
|   |-- game_room/
|   |   |-- game_room.go
|   |   `-- data_models.go
|   |-- protobufs/
|   |   |-- assets/
|   |   |   |-- client_server_message.proto
|   |   |   |-- server_client_message.proto
|   |   |   |-- chat_message_payload.proto
|   |   |   |-- game_message_payload.proto
|   |   |   |-- other_payloads.proto
|   |   |   `-- player_data.proto
|   |   `-- *.pb.go
|   `-- web_socket/
|       |-- websocket_handler.go
|       |-- handle_all_message.go
|       |-- handle_player_info.go
|       `-- handle_messages/
|           |-- handle_chat_message.go
|           |-- handle_game_state.go
|           |-- handle_game_update.go
|           `-- response_message_types/
|               |-- send_message.go
|               `-- broadcast_message.go
`-- tests/
    `-- test_client.go
```

## Package Responsibilities

`main.go`

- Owns process startup and HTTP server.
- Owns the global room registry.
- Creates rooms through `/create`.
- Does not contain message handling logic.

`internal/game_room`

- Defines room and player connection data structures.
- `GameRoom` stores `RoomID`, `PlayerData`, `PlayersMux`, and `LastActivity`.
- Keep shared room state here or in closely related files.

`internal/protobufs/assets`

- Source of truth for message contracts.
- Update these `.proto` files before changing generated Go protobuf code.
- Regenerate `internal/protobufs/*.pb.go` after proto edits.

`internal/web_socket`

- Owns WebSocket route registration, connection upgrade, connect ACK, player id generation, and inbound message dispatch.

`internal/web_socket/handle_messages`

- Owns per-message business behavior.
- Chat is implemented.
- Game state and game update are stubs.

`internal/web_socket/handle_messages/response_message_types`

- Owns protobuf marshaling and WebSocket writes for one-client send and room broadcast.

`tests/test_client.go`

- Manual CLI client for creating/joining rooms and sending chat messages.
- It is not a Go test file despite living in `tests`.

## Protocol Contract

Client to server message types:

- `REQUEST_GAME_STATE`
- `SEND_GAME_UPDATE`
- `SEND_CHAT_MESSAGE`

Server to client message types:

- `RESPOND_GAME_STATE`
- `BROADCAST_GAME_UPDATE`
- `BROADCAST_CHAT_MESSAGE`
- `SEND_ON_CONNECT_ACK`

Payloads:

- `ChatMessagePayload`: `sender_id`, `content`
- `GameStatePayload`: `level`, `is_started`, `is_over`, `currentLetter`
- `GameUpdatePayload`: `level`, `type`
- `OnConnectAckPayload`: `roomId`, `playerId`
- `PlayerData`: `player_id`, `name`, `hearts`, `score`

Important protocol notes:

- Frames are WebSocket binary messages.
- Payloads are protobuf, not JSON.
- The frontend must use a protobuf-capable plain WebSocket client to talk to this backend.
- `OnConnectAckPayload` currently uses camelCase proto field names (`roomId`, `playerId`) while several other messages use snake_case. Preserve current generated accessors unless intentionally normalizing the schema.

## Concurrency and State

Current state model:

- `gameRooms` is a process-level `map[string]*GameRoom` guarded by `gameRoomsMux`.
- `GameRoom.PlayerData` is guarded by `GameRoom.PlayersMux`.
- Each WebSocket connection reads messages in its own handler loop.
- Broadcast locks `PlayersMux` while marshaling and writing to each connection.

Agent cautions:

- Do not read or write `GameRoom.PlayerData` without considering `PlayersMux`.
- Avoid holding `PlayersMux` across slow operations if expanding broadcast behavior.
- Add disconnect cleanup before relying on player counts or room lifecycle.
- Dynamic `http.HandleFunc("/"+roomID, ...)` works for the prototype, but route registration is global and permanent for the process.

## Style and Conventions

Code style:

- Standard Go formatting with `gofmt`.
- Small packages grouped by responsibility.
- Exported functions are used where package boundaries require them.
- Existing code favors direct structs and simple functions over interfaces.

Naming:

- Package directories use lowercase with underscores where already present.
- Message handler functions follow `HandleX` naming.
- Response helpers use `SendMessage` and `BroadcastMessage`.

Error handling:

- Current code generally logs with `fmt.Println` or `log.Println` and continues.
- For production paths, prefer returning errors where callers can act on them.

Comments:

- Keep comments short and useful.
- Existing TODOs indicate prototype gaps; update them when implementing the relevant behavior.

## Common Change Patterns

Add a new client message:

1. Add enum value and payload field to `client_server_message.proto`.
2. Add or update payload schema in an appropriate `.proto` file.
3. Regenerate Go protobuf files.
4. Add a `case` in `HandleAllMessage`.
5. Implement handler in `internal/web_socket/handle_messages`.
6. Add a send path in the frontend protocol client.

Add a new server message:

1. Add enum value and payload field to `server_client_message.proto`.
2. Regenerate Go protobuf files.
3. Create response in a handler.
4. Send with `SendMessage` or `BroadcastMessage`.
5. Decode and handle it on the frontend.

Implement authoritative game state:

1. Extend `GameRoom` with state fields or a nested state struct.
2. Protect state with the same mutex or a clearly documented separate mutex.
3. Make `REQUEST_GAME_STATE` return `RESPOND_GAME_STATE`.
4. Make `SEND_GAME_UPDATE` validate, mutate state, and broadcast `BROADCAST_GAME_UPDATE`.
5. Add tests or a small integration harness for message ordering and validation.

## Regenerating Protobufs

Use the command from `README.md` after editing proto files:

```bash
protoc --proto_path=internal/protobufs/assets --go_out=internal/protobufs --go_opt=paths=source_relative client_server_message.proto server_client_message.proto player_data.proto other_payloads.proto game_message_payload.proto chat_message_payload.proto
```

Generated files live in `internal/protobufs`. Do not manually edit generated `.pb.go` files.

## Running and Verification

Run the backend:

```bash
go run .
```

Run tests/checks:

```bash
go test ./...
```

Manual client:

```bash
go run tests/test_client.go
```

Expected local endpoints:

- `GET http://localhost:8080/create`
- `ws://localhost:8080/{roomId}`

## Known Gaps

- No room cleanup or expiry.
- No disconnect cleanup.
- No room join validation beyond the dynamic route existing.
- No CORS/origin restrictions.
- Game state and game update handlers are stubs.
- Broadcast writes happen while holding the player mutex.
- Test client is manual and not wired into `go test`.
- The frontend currently uses `socket.io-client`, which is not compatible with this backend's plain WebSocket protocol.


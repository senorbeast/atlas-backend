# Atlas Backend AI Agent Guide

`atlas-backend` is the authoritative multiplayer service for Atlas rooms. It owns room lifecycle, player identity, chat history, turn sequencing, scoring, game snapshots, and Atlas city validation.

## Backend Architecture

```mermaid
flowchart TD
  Main["main.go<br/>HTTP routes"]
  WS["internal/web_socket<br/>WebSocket upgrade and dispatch"]
  Manager["RoomManager<br/>room registry and cleanup"]
  Room["GameRoom<br/>one room's mutable state"]
  Chat["ChatHistory<br/>messages + pagination"]
  Turns["TurnPolicy<br/>strict/free-for-all"]
  Score["ScorePolicy<br/>+1 accepted city"]
  Game["games.Game interface"]
  Atlas["AtlasWordGame<br/>city validation/history"]
  Proto["internal/protobufs<br/>generated transport structs"]

  Main --> WS
  Main --> Manager
  WS --> Manager
  WS --> Proto
  Manager --> Room
  Room --> Chat
  Room --> Turns
  Room --> Score
  Room --> Game
  Game --> Atlas
  Room --> Proto
```

## Routes

- `GET /create?gameKind=atlas-word&turnMode=strict-turns`: creates an in-memory room and returns room metadata.
- `GET /rooms/{roomId}/ws`: upgrades to a plain WebSocket using binary protobuf frames.
- `GET /rooms/{roomId}/chat?limit=50&before=<messageId>`: returns room-scoped chat history latest-first.

`ATLAS_PORT` overrides the default `8080`.

## WebSocket Flow

```mermaid
sequenceDiagram
  participant C as Client
  participant WS as WebSocket Handler
  participant R as GameRoom
  participant G as Game

  C->>WS: Connect /rooms/{id}/ws
  C->>WS: JOIN_ROOM(display_name, game_kind, turn_mode)
  WS->>R: JoinPlayer
  R-->>WS: Player + room/game snapshot
  WS-->>C: SEND_ON_CONNECT_ACK
  C->>WS: SEND_CHAT_MESSAGE(content)
  WS->>R: AddChatMessage using connection identity
  WS-->>C: BROADCAST_CHAT_MESSAGE
  C->>WS: SEND_GAME_UPDATE(submit_city)
  WS->>R: ApplyPlayerAction
  R->>G: Validate/apply move
  WS-->>C: BROADCAST_GAME_UPDATE
  WS-->>C: RESPOND_GAME_STATE
  WS-->>C: BROADCAST_ROOM_UPDATE
```

Rejected joins or moves send typed `SEND_ERROR` messages. Rejected moves must not mutate game history, score, turn, or player metadata.

## Package Responsibilities

- `main.go`: route registration, JSON responses, room creation endpoint, cleanup loop.
- `internal/game_room`: `RoomManager`, `GameRoom`, player metadata, chat history, turn policy, scoring, room/game snapshots, cleanup and expiry.
- `internal/games`: generic `Game` interface and `atlas-word` implementation.
- `internal/protobufs/assets`: `.proto` source of truth.
- `internal/protobufs`: generated Go protobuf files; do not edit manually.
- `internal/web_socket`: WebSocket upgrade, join handshake, protobuf dispatch, response/broadcast helpers.
- `tests/test_client.go`: manual client, not part of `go test`.

## Game Room State

```mermaid
classDiagram
  class RoomManager {
    +CreateRoom()
    +GetRoom()
    +ChatPage()
    +CleanupExpired()
  }
  class GameRoom {
    +JoinPlayer()
    +DisconnectPlayer()
    +AddChatMessage()
    +ApplyPlayerAction()
    +Snapshot()
  }
  class Game {
    <<interface>>
    +Kind()
    +Snapshot()
    +Apply()
    +ExpiresAt()
    +IsOver()
  }
  class AtlasWordGame {
    +Apply()
    +Snapshot()
  }
  RoomManager "1" --> "*" GameRoom
  GameRoom --> Game
  Game <|.. AtlasWordGame
```

Room state is in memory and guarded by mutexes. Do not access player, chat, or game state without using the room/manager methods or matching lock discipline.

## Protocol Contract

Client to server:

- `JOIN_ROOM`
- `SEND_CHAT_MESSAGE`
- `REQUEST_GAME_STATE`
- `SEND_GAME_UPDATE`

Server to client:

- `SEND_ON_CONNECT_ACK`
- `BROADCAST_CHAT_MESSAGE`
- `BROADCAST_GAME_UPDATE`
- `RESPOND_GAME_STATE`
- `BROADCAST_ROOM_UPDATE`
- `SEND_ERROR`

Accepted move event ordering is fixed:

```mermaid
flowchart LR
  A["BROADCAST_GAME_UPDATE"] --> B["RESPOND_GAME_STATE"] --> C["BROADCAST_ROOM_UPDATE"]
```

## Adding Backend Features

For protocol changes, edit `internal/protobufs/assets/*.proto`, regenerate Go protobufs, regenerate frontend protobufs, then update handlers and tests.

For new games, implement `games.Game`, add the `gameKind` to `games.NewGame`, provide action validation, snapshot data, completion rules, and any game-specific scoring or turn behavior. See `FUTURE_GAMES.md`.

## Verification

```powershell
$env:GOCACHE = "D:\Huehue\atlas\.gocache"
go test ./...
go test -race ./...
```

The race test requires a local C compiler. CI runs backend tests from `.github/workflows/ci.yml`.

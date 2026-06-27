# Atlas Backend

Go backend for Atlas multiplayer rooms, chat, turn sequencing, scoring, and authoritative Atlas city validation.

## Run

```bash
go run .
```

The server exposes:

- `GET /create`
- `GET /rooms/{roomId}/ws`
- `GET /rooms/{roomId}/chat?limit=50&before=<messageId>`

## Test

PowerShell with a workspace-local cache:

```powershell
$env:GOCACHE = "D:\Huehue\atlas\.gocache"
go test ./...
go test -race ./...
```

The regression suite covers room creation/join snapshots, player-name validation, strict turns, round-robin advancement, score sync, disconnect turn reassignment, free-for-all mode, chat pagination, chat JSON shape, city validation, duplicate/wrong-letter rejection, expiry, empty-room cleanup, WebSocket join/chat/move flows, event ordering, and late-join snapshots.

## Generate Protobufs

```bash
protoc --proto_path=internal/protobufs/assets --go_out=internal/protobufs --go_opt=paths=source_relative client_server_message.proto server_client_message.proto player_data.proto other_payloads.proto game_message_payload.proto chat_message_payload.proto
```

## Structure

- `internal/game_room`: room lifecycle, players, chat history, turn helpers, scoring policy, snapshots.
- `internal/games`: generic game interface and Atlas word-chain implementation.
- `internal/protobufs`: generated Go protobuf contracts.
- `internal/web_socket`: websocket join and message handlers.
- `FUTURE_GAMES.md`: extension guide for adding new games to the multiplayer loop.

## Architecture Docs

- `../ARCHITECTURE.md`: cross-project diagrams and runtime flows.
- `AI_AGENT_GUIDE.md`: backend package boundaries, WebSocket flow, and protocol event ordering.
- `FUTURE_GAMES.md`: reusable multiplayer loop and new-game extension diagrams.

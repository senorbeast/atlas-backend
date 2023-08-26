
# Atlas Backend

## Architecture



### Run the Project

```bash

# Generate required files

# To run the app
go run *.go

go build -o ./bin        # builds binary in ./bin
go install               # installs app in $GOBIN or $GOPATH of the system.

```

#### Generating files

```bash
protoc --proto_path=internal/protobufs --go_out=internal/protobufs --go_opt=paths=so
urce_relative internal/protobufs/game.proto
```

<!-- protoc -I=src/protobuf/ --go_out=src/protobuf/ src/protobuf/game.proto -->

#### Folder structure

```bash
├── bin
│   └── atlas-backend
├── go.mod  
├── go.sum
├── internal        # internal packages
│   ├── game_room
│   │   ├── data_models.go
│   │   └── game_room.go
│   ├── protobufs   # protobuf defs and gens
│   │   ├── game.pb.go
│   │   └── game.proto
│   └── web_socket
│       └── websocket_handler.go
├── main.go         # entry point
├── packages        # exportable packages 
├── README.md
└── tests
    └── test_client.go
```
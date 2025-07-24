# MCNode - Minecraft Node Server

A Go HTTP server to manage Minecraft Docker containers for your control panel.

## Features

- Secured by Bearer token authentication on every request
- /handshake endpoint to verify token
- /server/start endpoint creates or starts a Minecraft container with unique name per user
- Uses Docker to run Minecraft servers with volumes per server inside `volume/<container-name>`
- Unique container names: `<server-name>-<userId>` where `userId` is email username part

## Getting Started

### Prerequisites

- Go 1.18 or newer
- Docker installed and runnable by current user
- Your `.env` file with `HANDSHAKE_TOKEN`

### Setup

1. Clone this repo on your Minecraft VPS.

2. Copy `.env.example` to `.env` and edit the token:
```bash
cp env.example env 
nano .env
```

3. Download dependencies:
```bash
go mod tidy
```

4. Build the binary:
```bash
go build -o mcnode .
```

5. Run the server:
```bash
./mcnode
```


You should see:
- Node HTTP server listening on :25575



### API Endpoints

All endpoints require HTTP header:
- Authorization: Bearer your-actual-token
Content-Type: application/json


#### POST /handshake

Simple endpoint to verify token

- Request body: `{}` or empty allowed
- Response: `{ "status": "ok" }`

#### POST /server/start

Start or create a Minecraft server container with unique naming:

- Request JSON body:
```
{
"serverName": "lobby",
"userEmail": "alice@example.com",
"hostPort": "25565" // (optional) port to expose on VPS
}
```

- Response example:
```
{
"status": "ok",
"message": "Server started successfully"
}
```

### Folder Structure

- Server data stored in `volume/<server-name>-<userId>/`
- Each container mounts its folder to `/data` inside Docker container

### Next Steps

- Implement `/server/stop`, `/server/restart`
- Add file management API endpoints
- Enhance error handling and port management

---

Keep your `.env` secret and secure. Use firewall or network rules to restrict access to port `25575`.


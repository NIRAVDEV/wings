# MCNode - Minecraft Node Server

This is the Go-based node server for your Minecraft control panel. It verifies handshake tokens and will manage Docker Minecraft server containers in future versions.

## Getting Started

### Prerequisites

- Go (1.18 or later recommended)
- Docker installed on the node VPS
- Git (optional for clone)

### Setup

1. Clone or download this repository on your Minecraft server VPS.

2. Install dependencies and initialize Go module:


3. Copy the example env file and set your handshake token:
cp .env.example env file
nano .env
- Add the token in HANDSHAKE_TOKEN

4. Build the server binary:
'''bash
go build -o mcnode

6. Run the node server:
./mcnode


### API Usage

The node server listens on port `25575` and exposes the following endpoint so far:

#### POST `/handshake`

Verifies the token sent in the `Authorization` header using the Bearer scheme.

- Request Headers:

- Request Body:

Empty JSON or `{}` is acceptable.

- Response:
{"status": "ok"}


- HTTP 401 if token invalid or missing.

### Next Steps

Future endpoints will implement commands such as:

- Start / Stop / Restart Minecraft servers inside Docker containers
- File management for server files

All endpoints will require the same Bearer token in the `Authorization` header.

---

### Notes

- Keep your `.env` secret and never commit it to version control!
- Always run the node behind a firewall or restrict access to trusted IPs.
- Secure communication using TLS/proxy for production.

---

If you have any questions or need help, just ask!

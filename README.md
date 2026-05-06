# MangaHub - Manga & Comic Tracking System

**Course:** Net-centric Programming - IT096IU  
**Instructors:** Lê Thanh Sơn - Nguyễn Trung Nghĩa  
**Programming Language:** Go  

MangaHub is a comprehensive command-line and network-centric manga tracking system. It allows users to discover manga, track reading progress in real-time across multiple devices, and participate in community discussions.

This project was built to demonstrate proficiency in modern network programming by integrating **five distinct network protocols** concurrently in a single Go application.

## 🚀 System Architecture & Protocols

The MangaHub backend seamlessly orchestrates five protocols:
1. **HTTP/REST (Port 8080):** Core API using `gin-gonic` for user authentication (JWT), library management, and database CRUD operations.
2. **TCP (Port 9090):** Stateful real-time synchronization server for broadcasting reading progress across connected devices.
3. **UDP (Port 9091):** Lightweight, connectionless broadcasting system for pushing new chapter notifications to subscribed clients.
4. **WebSocket (Port 9093):** Real-time, bidirectional chat hubs (`gorilla/websocket`) for global and manga-specific community discussions.
5. **gRPC (Port 9092):** High-performance internal Remote Procedure Call system using Protocol Buffers for fast, typed microservice communication.

## 🛠️ Prerequisites

* **Go** 1.19 or higher
* **GCC / C-Compiler** (Required for CGO to build the `mattn/go-sqlite3` database driver)
* **SQLite3**

## 📦 Installation & Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/yourorg/mangahub.git
   cd mangahub
   ```

2. **Download dependencies:**
   ```bash
   go mod tidy
   ```

3. **Build the Server and CLI:**
   ```bash
   # Build the main server
   go build -o mangahub-server ./cmd/mangahub-server
   
   # Build the CLI application
   go build -o mangahub ./cmd/mangahub-cli
   ```

## 🎮 Quick Start Usage

### 1. Start the Server
Run the server orchestrator, which spins up all 5 protocol servers simultaneously:
```bash
./mangahub-server
```

### 2. Use the CLI
Open a new terminal window to interact with the system via the CLI.

**Authentication & Search:**
```bash
./mangahub auth register --username myuser --email myuser@example.com
./mangahub auth login --username myuser
./mangahub manga search "one piece"
```

**Library & Progress Tracking (Triggers HTTP & TCP):**
```bash
./mangahub library add --manga-id one-piece --status reading
./mangahub progress update --manga-id one-piece --chapter 1095
```

**Real-time Features (TCP, UDP, WebSocket):**
```bash
./mangahub sync monitor          # Listen to TCP progress syncs
./mangahub notify subscribe      # Listen to UDP notifications
./mangahub chat join             # Connect to WebSocket chat
```

## 📁 Project Structure

```text
mangahub/
├── cmd/
│   ├── mangahub-server/    # Server orchestrator (main entry point)
│   └── mangahub-cli/       # Command-line interface application
├── internal/
│   ├── auth/               # JWT authentication logic
│   ├── http/               # REST API & routing (Gin)
│   ├── tcp/                # Progress sync server
│   ├── udp/                # Notification broadcaster
│   ├── websocket/          # Real-time chat hubs
│   └── grpc/               # Internal gRPC services
├── pkg/
│   ├── models/             # Shared data structures (JSON/DB schemas)
│   └── database/           # SQLite database initialization and repositories
├── proto/                  # Protocol Buffer definitions
└── data/                   # Initial manga JSON data (Database seeding)
```

## 📝 License & Academic Integrity
This project is submitted for academic grading. AI tools were utilized strictly within the permitted guidelines outlined in the project documentation for brainstorming, boilerplate generation, and debugging assistance. All core architectures and final implementations are the original work of the student team.

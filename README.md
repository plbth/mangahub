# MangaHub - Manga & Comic Tracking System

**Course:** Net-centric Programming - IT096IU  
**Instructors:** Lê Thanh Sơn - Nguyễn Trung Nghĩa  
**Programming Language:** Go  

MangaHub is a comprehensive command-line and network-centric manga tracking system. It allows users to discover manga, track reading progress in real-time across multiple devices, and participate in community discussions.

This project was built to demonstrate proficiency in modern network programming by integrating **five distinct network protocols** concurrently in a single Go application.

## System Architecture & Protocols

The MangaHub backend seamlessly orchestrates five protocols:
1. **HTTP/REST (Port 8080):** Core API using `gin-gonic` for user authentication (JWT), library management, and database CRUD operations.
2. **TCP (Port 9090):** Stateful real-time synchronization server for broadcasting reading progress across connected devices.
3. **UDP (Port 9091):** Lightweight, connectionless broadcasting system for pushing new chapter notifications to subscribed clients.
4. **WebSocket (Port 9093):** Real-time, bidirectional chat hubs (`gorilla/websocket`) for global and manga-specific community discussions.
5. **gRPC (Port 9092):** High-performance internal Remote Procedure Call system using Protocol Buffers for fast, typed microservice communication.

## Prerequisites

* **Go** 1.19 or higher
* **GCC / C-Compiler** (Required for CGO to build the `mattn/go-sqlite3` database driver)
* **SQLite3**

## Installation & Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/plbth/mangahub.git
   cd mangahub
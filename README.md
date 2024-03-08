<p align="center">
    <img src="https://github.com/WasinUddy/Montainer/blob/main/logo.png?raw=true" width="300">
</p>
<h1 align="center">Montainer</h1>

<p align="center">
Minecraft + Container = Montainer!!!<br>
Minecraft Bedrock Server easily deployed with Docker.
</p>

## Problem Statement

Updating a Minecraft server can be a tedious process. While clients can easily update their game via Microsoft Store or other platforms, server updates require manual download and replacement of files. This can be time-consuming and prone to errors.

## Solution

Montainer is designed to solve this problem. It is a Minecraft server encapsulated in a Docker container, complete with a web UI console. The repository is managed automatically by web scraping the Mojang website. This allows users to use the `:latest` tag and auto-deploying programs like Watchtower to ensure their server is up-to-date with the client, without the need for manual updates.

## How it Works

1. **Docker Container**: The Minecraft server is encapsulated in a Docker container, making it easy to deploy and manage.

2. **Web UI Console**: Montainer comes with a web UI console, providing a user-friendly interface for managing your server.

3. **Automatic Updates**: The repository is updated automatically by web scraping the Mojang website. This means you can always use the `:latest` tag to get the most recent version of the server. You can also use auto-deploying programs like Watchtower to ensure your server is always up-to-date.

## Usage

### Option 1: Deploy on Kubernetes (Recommended)
Look at the examples/kubernetes folder for a sample deployment.

### Option 2: Deploy with Docker Compose (Recommended)
```yaml
version: '3'
services:
  montainer:
    image: ghcr.io/wasinuddy/montainer-stable:latest  # Use montainer-preview for Minecraft Snapshot server
    
    ports:
      - "8000:8000"                                   # Web UI Console mount to port 8000 TCP
      - "19132:19132/udp"                             # Minecraft Bedrock Server port 19132 UDP
    
    volumes:
        - ./worlds:/app/minecraft_server/worlds       # Paste your world folder inside ./worlds (ie. ./worlds/Bedrock Level)
    
    restart: unless-stopped
```

## TODO
- [ ] Add Environment Variables for Game configuration
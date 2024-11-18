<h1 align="center">Montainer</h1>
<h3 align="center">Easily deployable Minecraft Bedrock Server in a Docker container with automatic updates</h3>

<p align="center">
  <img src="https://img.shields.io/github/stars/WasinUddy/Montainer?style=social" alt="GitHub stars">
  <img src="https://img.shields.io/github/forks/WasinUddy/Montainer?style=social" alt="GitHub forks">
  <img src="https://img.shields.io/github/issues/WasinUddy/Montainer" alt="GitHub issues">
</p>

---
## Problem Statement
Updating a Minecraft server can be tedious and error-prone. While players can easily update their game via Microsoft Store or other platforms, server admins have to manually download and replace files, risking inconsistencies and errors. Montainer simplifies this process.

## Solution
Montainer (Minecraft + Container) provides a self-contained Minecraft Bedrock server in a Docker container, streamlining updates through automated web scraping of Mojang's website. By using the `:latest` tag, users can always deploy the most recent server version with minimal effort.

## Features
1. **Docker Container**: The Minecraft server is encapsulated in a Docker container, making it easy to deploy and manage.
2. **Web UI Console**: Montainer comes with a web UI console, providing a user-friendly interface for managing your server.
3. **Automatic Updates**: The repository is updated automatically by web scraping the Mojang website. This means you can always use the `:latest` tag to get the most recent version of the server. You can also use auto-deploying programs like [watchtower](https://github.com/containrrr/watchtower) to ensure your server is always up-to-date.
4. **Volume Mounting**: Montainer allows you to store world data and configuration files on the host, enabling easy backup, restore, and migration.
5. **Subpath Support**: Set unique subpaths for multiple servers on a single host, so you can manage them independently within the same Docker environment.

<figure>
  <img src="https://raw.githubusercontent.com/WasinUddy/Montainer/main/images/webui.png" 
       alt="A screenshot of the Montainer web user interface showing key functionalities and layout." 
       style="width:100%;max-width:600px;">
  <figcaption>A screenshot of the Montainer WebUI, showcasing its interface and features.</figcaption>
</figure>

## TODO
- [ ] Add a Backup button in the web UI console to back up server data to AWS S3.
- [ ] Implement Command autofill in the web UI console.
- [ ] Add Log export functionality to integrate with log aggregation services.

## Usage

1. **Deploy Montainer with Docker (recommended):**

   ```yaml
   services:
     montainer:
       image: ghcr.io/wasinuddy/montainer-stable:latest # 'montainer-preview' for Snapshot server
       ports:
         - "8000:8000"      # Web UI console on port 8000
         - "19132:19132/udp" # Minecraft Bedrock server port
       volumes:
         - ./worlds:/app/instance/worlds # Mount for world data
         - ./configs:/app/configs       # Mount for server configurations
       restart: unless-stopped
   ```

2. **Access the web UI console**:
   Visit `http://localhost:8000` in your browser. Use the Start/Stop button to control the server directly from the console.

3. **Kubernetes Deployment** 🚢
   Montainer can also be deployed on Kubernetes. Ensure your Ingress Controller supports WebSocket, as both HTTP and WebSocket are used for the web UI console.

---
## Contributing
Contributions are welcome! Feel free to open issues or submit pull requests to enhance Montainer.

---

<p align="center">
This project is dedicated to the great memories shared with friends while playing Minecraft. Montainer is built to make server management as seamless as the fun we’ve had exploring and building together.
</p>


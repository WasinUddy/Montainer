<p align="center">
  <img src="https://github.com/WasinUddy/Montainer/blob/main/resources/logo.png?raw=true" alt="Montainer Logo" width="200"/>
</p>

---

### Minecraft + Container = Montainer !!!

Since in order to play Minecraft Bedrock Edition the client must be the same version as the server, updating the client is super easy via the Microsoft Store, but updating the server is super complicated since it requires downloading a new binary and replacing the old one. This project puts the server in a container so that it can be updated easily by mounting the world and configuration files as volumes and easily updating the container.


## Features
1. Containerized Minecraft Bedrock Edition Server
2. Web UI for managing the server no need to interact with the container directly
3. Self Managing Repositories automatically update the container when a new version is released by scraping from the Mojang website
4. Thick image version which contained the server inside the image

## Usage
There are two ways to use this project, either run it directly from docker or use the docker-compose which is the recommended way. You may use it with services like Watchtower to automatically update the container when a new version is released so you can enjoy minecraft without worrying about updating the server.

### Sample docker-compose.yaml

```yaml
version: "3.8"

services:
  montainer:
    image: ghcr.io/wasinuddy/montainer:latest
    container_name: montainer
    restart: unless-stopped
    ports:
      - 19132:19132/udp
      - 8000:8000
    volumes:
      - ./allowlist.json:/minecraft/minecraft_server/allowlist.json
      - ./server.properties:/minecraft/minecraft_server/server.properties
      - ./level:/minecraft/minecraft_server/worlds/Bedrock level
```
Access the Web UI via http://localhost:8000 and game via localhost:19132

## TODO
-  VMWare vCenter inspired vTainer server to cluster multiple server 
-  Add support for preview versions


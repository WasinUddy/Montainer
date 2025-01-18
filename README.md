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
- [x] Add a Backup button in the web UI console to back up server data to AWS S3.
- [ ] Implement Command autofill in the web UI console.
- [ ] Add Log export functionality to integrate with log aggregation services.

# Montainer Deployment Guide

## Docker Deployment (Recommended)

### Platform-Specific Instructions

#### For Native AMD64 and Apple Silicon
```yaml
services:
  montainer:
    image: ghcr.io/wasinuddy/montainer-stable:latest  # Use 'montainer-preview' for Snapshot server
    ports:
      - "8000:8000"       # Web UI console
      - "19132:19132/udp" # Minecraft Bedrock server
    volumes:
      - ./worlds:/app/instance/worlds  # World data
      - ./configs:/app/configs         # Server configurations
    restart: unless-stopped
```

#### For ARM64 Machines  (tested on Ampere Altra A1)
> **Note**: The official Mojang binary does not support ARM64 architecture. However, you can run the Docker image on ARM64 machines using Linux Kernel **binfmt** for architecture emulation. which may lead to performance issues.

```yaml
services:
  binfmt:
    image: tonistiigi/binfmt
    privileged: true
    command: --install all
    restart: "no"
  
  montainer:
    image: ghcr.io/wasinuddy/montainer-stable:latest  # Use 'montainer-preview' for Snapshot server
    platform: linux/amd64  # Explicitly specify platform
    ports:
      - "8000:8000"       # Web UI console
      - "19132:19132/udp" # Minecraft Bedrock server
    volumes:
      - ./worlds:/app/instance/worlds  # World data
      - ./configs:/app/configs         # Server configurations
    restart: unless-stopped
    depends_on:
      binfmt:
        condition: service_completed_successfully
```



## Accessing the Server

1. Open your web browser and navigate to `http://localhost:8000`
2. Use the Web UI console to start/stop the server and manage settings

## Kubernetes Deployment 🚢

Montainer supports Kubernetes deployment with the following requirements:
- Your Ingress Controller must support WebSocket connections
- Both HTTP and WebSocket protocols are used for the web UI console

> For detailed Kubernetes deployment instructions, please refer to our Kubernetes documentation.

## Environment Variables

| **Environment Variable**     | **Description**                                                                                                                            | **Default Value**   |
|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------|---------------------|
| `SUBPATH_URL`                 | The subpath URL used to access Montainer's web UI. If set, Montainer will be accessible at the path `http://localhost:8000/{SUBPATH_URL}`. | `/`                 |
| `AWS_S3_ENDPOINT`             | The endpoint for AWS S3 or compatible storage service. This is required for backup operations. empty string for disable s3 back up         | (empty string)      |
| `AWS_S3_KEY_ID`              | The AWS access key ID for authentication with the S3 service.                                                                              | (empty string)      |
| `AWS_S3_SECRET_KEY`           | The AWS secret access key for authentication with the S3 service.                                                                          | (empty string)      |
| `AWS_S3_BUCKET_NAME`          | The name of the S3 bucket where backup data will be stored.                                                                                | (empty string)      |
| `AWS_S3_REGION`               | The AWS region where the S3 bucket is located. This is needed for connecting to S3.                                                        | (empty string)      |
| `INSTANCE_NAME`               | The name of the Montainer instance. This is used to uniquely identify and label your Montainer instance.                                   | `Montainer`         |


---
## Contributing
Contributions are welcome! Feel free to open issues or submit pull requests to enhance Montainer.

---

<p align="center">
This project is dedicated to the great memories shared with friends while playing Minecraft. Montainer is built to make server management as seamless as the fun we’ve had exploring and building together.
</p>


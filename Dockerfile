FROM python:3.11-slim

# Image Label
LABEL org.opencontainers.image.source https://github.com/wasinuddy/montainer

# Set the working directory
WORKDIR /app

# Install Dependencies for downloading and executing minecraft-bedrock-server
RUN apt-get update && apt-get install -y wget unzip libcurl4

# Install python dependencies
COPY requirements.txt requirements.txt
RUN pip3 install -r requirements.txt

# Copy minecraft-bedrock-server version and downloader
COPY versions.json versions.json
COPY scripts/download_minecraft_server.py scripts/download_minecraft_server.py

# Expose the port
EXPOSE 19132/udp
EXPOSE 8000

# Copy webui
RUN mkdir web
COPY web/backend web


ENTRYPOINT [ "python3", "web/main.py" ]
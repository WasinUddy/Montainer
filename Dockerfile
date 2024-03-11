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

# Expose the port
EXPOSE 19132/udp
EXPOSE 8000

# Copy webui
RUN mkdir web
COPY web/backend web

# Copy the server files
COPY minecraft_server /app/minecraft_server

# Create config directory for game configuration
RUN mkdir /app/config
# Symbolic link to the config file of concern
RUN ln -s /app/minecraft_server/server.properties /app/config/server.properties 
RUN ln -s /app/minecraft_server/allowlist.json /app/config/allowlist.json


ENTRYPOINT [ "python3", "web/main.py" ]
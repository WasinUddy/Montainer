# Use the slim version of Python 3.11 as the base image
FROM python:3.11-slim

# Image label indicating the source of the Dockerfile
LABEL org.opencontainers.image.source=https://github.com/wasinuddy/montainer

# Set the working directory inside the container to /app
WORKDIR /app

# Install dependencies required for downloading and executing the Minecraft Bedrock server
RUN apt-get update && apt-get install -y wget unzip libcurl4 zip && apt-get clean && rm -rf /var/lib/apt/lists/*

# Remove Cache and unnecessary packages
RUN apt-get clean && rm -rf /var/lib/apt/lists/*

# Copy the requirements.txt file into the container and install Python dependencies
COPY requirements.txt requirements.txt
RUN pip3 install -r requirements.txt

# Expose ports for the Minecraft server and the web application
EXPOSE 19132/udp
EXPOSE 8000

# Create a directory for the web UI and copy the backend files into it
RUN mkdir web
COPY web/backend web

# Copy the Minecraft server files into the container
COPY minecraft_server /app/minecraft_server

# Create a directory to host configuration files
RUN mkdir /app/configs

# Set the entry point for the container, using the architecture specified by the build argument
ENTRYPOINT [ "python3", "web/main.py", "--arch", "${ARCH}"]

# Use a smaller base image for Python 3.12
FROM --platform=linux/amd64 python:3.12-slim AS base

# Metadata about the image
LABEL authors="WasinUddy"

# Set the working directory
WORKDIR /app

# Install system dependencies in one layer to reduce image size
RUN apt-get update \
    && apt-get install -y \
    wget \
    unzip \
    libcurl4 \
    zip \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies in one layer
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN rm requirements.txt

# Create necessary directories
RUN mkdir -p instance configs
RUN mkdir -p /app/instance/worlds # Create necessary directories for volume mounts
# TODO: Add mount points for behavior_packs, resource_packs, etc.

# Copy only the necessary files to the container
# This should ideally be optimized based on the actual structure of your project
COPY bedrock_server/ /app/instance
COPY backend/ /app/

# Expose the required port
EXPOSE 8000
EXPOSE 19132/udp

# Healthcheck to ensure the container is healthy
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 CMD wget --quiet --tries=1 --output-document=- http://0.0.0.0:8000/healthz || exit 1

# Define the entry point for the container
CMD ["python", "main.py"]
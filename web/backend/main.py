from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
from server import Server

import os
import argparse
import logging

# Configure logging to log only errors
logging.basicConfig(level=logging.ERROR)
logger = logging.getLogger("uvicorn.error")
logger.setLevel(logging.ERROR)

# Create a FastAPI instance
app = FastAPI()

# Mount static files directory
app.mount("/static", StaticFiles(directory="/app/web/build/static"), name="static")

# Enable CORS for all origins, methods and headers
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Create a Server instance with the specified working directory
minecraft_server = Server(cwd="/app/minecraft_server")

@app.get("/")
def get_index():
    # Serve the index.html file
    return FileResponse("/app/web/build/index.html")

@app.post("/start")
def start_server():
    # Start the Minecraft server
    try:
        minecraft_server.start()
        return {"message": "Server started"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/stop")
def stop_server():
    # Stop the Minecraft server and remove the log file
    try:
        minecraft_server.stop()
        os.remove("server.log")
        return {"message": "Server stopped"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/command")
def send_command(cmd: str):
    # Send a command to the Minecraft server
    try:
        if cmd=="stop":
            stop_server()
        else:
            minecraft_server.command(cmd)
        return {"message": f"Command '{cmd}' executed"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/log")
def get_log():
    # Return the server log
    if not os.path.exists("server.log"):
        return {"log": ""}
    try:
        with open("server.log", "r") as f:
            return {"log": f.read()}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


# Kubernetes health checks
@app.get("/health/readiness")
def readiness():
    # Ready when FastAPI server is running (ready to accept requests)
    return {"message": "ready"}

@app.get("/health/liveness")
def liveness():
    # Server is not in Illegal State
    if minecraft_server.live_probe():
        return {"message": "live"}
    else:
        raise HTTPException(status_code=500, detail="Server is in Illegal State")



if __name__ == "__main__":
    # edit index.html for correct subdirectory
    subpath = os.environ.get("SUBPATH", "")
    with open("/app/web/build/index.html", "r") as f:
        index_html = f.read()
    
    # Replace /static/ with /subpath/static/ 
    if subpath not in index_html:
        index_html = index_html.replace("/static/", f"{subpath}/static/")
    
    # Write the modified index.html back to the file
    os.remove("/app/web/build/index.html")
    with open("/app/web/build/index.html", "w") as f:
        f.write(index_html)


    # Start the Minecraft server
    start_server()

    # Start the FastAPI server
    import uvicorn

    # Get the port from the environment variable
    port = int(os.environ.get("WEBPORT", 8000))
    uvicorn.run(app, host="0.0.0.0", port=port)

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
from server import Server

import os
import argparse

# Parse command line arguments
parser = argparse.ArgumentParser(description='Start montainer server with specified architecture.')
parser.add_argument('--arch', type=str, help='Architecture type (e.g., linux/amd64, linux/arm64)')
args = parser.parse_args()

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
        minecraft_server.start(x86=(args.arch=="linux/amd64"))
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

if __name__ == "__main__":
    # edit index.html for correct subdirectory
    subpath = os.environ.get("SUBPATH", "")
    with open("/app/web/build/index.html", "r") as f:
        index_html = f.read()
    
    # Replace /static/ with /subpath/static/ 
    index_html = index_html.replace("/static/", f"{subpath}/static/")

    # Write the modified index.html back to the file
    os.remove("/app/web/build/index.html")
    with open("/app/web/build/index.html", "w") as f:
        f.write(index_html)


    # Start the Minecraft server
    start_server()

    # Start the FastAPI server
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
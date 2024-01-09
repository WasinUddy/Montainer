from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
from server import Server

import os

app = FastAPI()
app.mount("/static", StaticFiles(directory="/minecraft/webserver/build/static"), name="static")

# Allow CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

minecraft_server = Server(cwd="/minecraft/minecraft_server")

@app.get("/")
def get_index():
    return FileResponse("/minecraft/webserver/build/index.html")

@app.post("/start")
def start_server():
    try:
        minecraft_server.start()
        return {"message": "Server started"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/stop")
def stop_server():
    try:
        minecraft_server.stop()
        os.remove("server.log")
        return {"message": "Server stopped"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/command")
def send_command(cmd: str):
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
    if not os.path.exists("server.log"):
        return {"log": ""}
    try:
        with open("server.log", "r") as f:
            return {"log": f.read()}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))



if __name__ == "__main__":
    # if minecraft server is not downloaded, download it
    if not os.path.exists("/minecraft/minecraft_server"):
        print("Server not found, downloading...")
        server_type = os.environ.get('SERVER_TYPE', 'stable')
        os.system(f"python3 /minecraft/scripts/server_downloader.py --type {server_type}")
        
    # Initially start the server
    start_server()

    # Start the Web Server
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
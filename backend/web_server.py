from fastapi import FastAPI, WebSocket, WebSocketDisconnect, HTTPException, Depends, Request
from fastapi.responses import JSONResponse, FileResponse
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

import asyncio
import logging
from minecraft_server import MinecraftServer
from connection_manager import ConnectionManager
from settings import settings

app = FastAPI()
instance = MinecraftServer(cwd='./instance')

manager = ConnectionManager()

# CORS configuration
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)



# Helper function for verifying server status
async def verify_server_running():
    if not instance.is_running:
        raise HTTPException(status_code=400, detail={'status': 'error', 'message': 'Server is not running.'})

# Mount Static files
app.mount('/assets', StaticFiles(directory='./dist/assets'), name='assets')
@app.get('/')
async def get_index():
    return FileResponse('./dist/index.html')

# Endpoint to start the Minecraft server
@app.post("/start")
async def start_minecraft_server():
    if instance.is_running:
        raise HTTPException(status_code=400, detail={'status': 'error', 'message': 'Server is already running.'})
    try:
        await asyncio.to_thread(instance.start)
        return {'status': 'success', 'message': 'Server started successfully.'}
    except Exception as e:
        logging.error(f"Error starting server: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# Endpoint to retrieve server status
@app.get('/status')
async def get_minecraft_server_status():
    return JSONResponse(content={'status': 'success', 'is_running': instance.is_running})


# Endpoint to stop the Minecraft server
@app.post("/stop")
async def stop_minecraft_server(running: bool = Depends(verify_server_running)):
    try:
        await asyncio.to_thread(instance.stop)
        return {'status': 'success', 'message': 'Server stopped successfully.'}
    except Exception as e:
        logging.error(f"Error stopping server: {e}")
        raise HTTPException(status_code=500, detail=str(e))

# Endpoint to toggle the server state
@app.post('/toggle')
async def toggle_start_stop():
    try:
        if instance.is_running:
            return await stop_minecraft_server()
        return await start_minecraft_server()
    except Exception as e:
        logging.error(f'Error toggling server: {e}')
        raise HTTPException(status_code=500, detail=str(e))


# Endpoint to send a command to the Minecraft server
@app.post("/command")
async def send_command(request: Request, running: bool = Depends(verify_server_running)):
    data = await request.json()
    command = data.get('command')

    if not command:
        raise HTTPException(status_code=422, detail="Command is required.")

    try:
        await asyncio.to_thread(instance.send_command, command)
        return {'status': 'success', 'message': 'Command sent successfully.'}
    except Exception as e:
        logging.error(f"Error sending command: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# Endpoint to retrieve server logs
@app.get('/logs')
async def get_logs(max_lines: int = 31, running: bool = Depends(verify_server_running)):
    try:
        async with asyncio.Lock():  # Prevent concurrent file access
            with open('instance.log', 'r') as log_file:
                logs = log_file.readlines()[-max_lines:]
                lines = [line.strip() for line in logs]
        return {'status': 'success', 'logs': lines}
    except Exception as e:
        logging.error(f"Error reading logs: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# Endpoint to restart server
@app.post('/restart')
async def restart_server():
    try:
        await asyncio.to_thread(instance.stop, force_shutdown=True)
        await asyncio.to_thread(instance.start)
        return {'status': 'success', 'message': 'Server restarted successfully.'}
    except Exception as e:
        logging.error(f"Error restarting server: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# Endpoint to Get instance name
@app.get('/instance_name')
async def get_instance_name():
    return {'instance_name': settings.INSTANCE_NAME}


# Endpoint to save persistent data to AWS S3
@app.post('/save')
async def save_data():
    if settings.AWS_S3_ENDPOINT == '':
        raise HTTPException(status_code=500, detail={'status': 'error', 'message': 'AWS S3 settings not configured.'})

    try:
        await asyncio.to_thread(instance.save_data)
        return {'status': 'success', 'message': 'Data saved successfully.'}
    except Exception as e:
        logging.error(f"Error saving data: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# WebSocket endpoint for data streaming
@app.websocket('/ws/stream')
async def websocket_endpoint(websocket: WebSocket):
    await manager.connect(websocket)
    try:
        while True:
            data = {
                'logs': [],
                'is_running': instance.is_running,
            }

            if instance.is_running:
                try:
                    with open('instance.log', 'r') as log_file:
                        logs = log_file.readlines()[-31:]
                        data['logs'] = [line.strip() for line in logs]
                except Exception as e:
                    logging.error(f"Error reading logs: {e}")

            await websocket.send_json(data)
            await asyncio.sleep(0.5)

    except WebSocketDisconnect:
        manager.disconnect(websocket)
        logging.info("Client disconnected from stream")
    except Exception as e:
        logging.error(f"WebSocket error: {e}")
        manager.disconnect(websocket)

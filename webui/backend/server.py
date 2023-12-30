import subprocess
import os

class Server:
    def __init__(self, cwd: str):
        self.cwd = cwd
        self.server = None

    def start(self):
        if not self.server:
            
            # Open log file if it doesn't exist
            if not os.path.exists("server.log"):
                open("server.log", "w").close()

            # Open un locking server.log
            self.server_log = open("server.log", "r+")

            self.server = subprocess.Popen(
                ["./bedrock_server"],
                cwd=self.cwd,
                stdout=self.server_log,
                stderr=self.server_log,
                stdin=subprocess.PIPE,
                text=True,
                bufsize=1,
                universal_newlines=True
            )

    def stop(self, force: bool = False):

        if self.server:
            if not force:
                self.command("stop")
                self.server.communicate()
                self.server = None
            else:
                self.server.kill()
                self.server = None

    def command(self, cmd: str):
        if self.server:
            self.server.stdin.write(f"{cmd}\n")
            self.server.stdin.flush()
        else:
            raise Exception("Server is not running")

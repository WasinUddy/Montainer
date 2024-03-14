import subprocess
import os

class Server:
    def __init__(self, cwd: str):
        """
        Constructor for the Server class.

        :param cwd: The current working directory where the server will be started.
        """
        self.cwd = cwd
        self.server = None
        self.config_files = ("server.properties", "allowlist.json")

    def start(self, x86: bool = True):
        """
        Starts the server process if it's not already running.
        """
        if not self.server:
            # Applied config files if empty initialize it with game files
            for file in self.config_files:
                if not os.path.exists(f"/app/configs/{file}"):
                    # File does not exist, copy it from minecraft_server
                    os.system(f"cp /app/minecraft_server/{file} /app/configs/{file}")
                else:
                    # File exists, copy it to minecraft_server and overwrite it
                    os.system(f"cp /app/configs/{file} /app/minecraft_server/{file}")


            # Check if the log file exists, if not, create it.
            if not os.path.exists("server.log"):
                open("server.log", "w").close()

            # Open the log file in read and write mode.
            self.server_log = open("server.log", "r+")

            # Start the server process, redirecting stdout and stderr to the log file.
            if x86:
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
            else:
                self.server = subprocess.Popen(
                    ["qemu-x86_64", "./bedrock_server"],
                    cwd=self.cwd,
                    stdout=self.server_log,
                    stderr=self.server_log,
                    stdin=subprocess.PIPE,
                    text=True,
                    bufsize=1,
                    universal_newlines=True
                )
                

    def stop(self, force: bool = False):
        """
        Stops the server process.

        :param force: If True, the server process is forcibly killed. If False, a "stop" command is sent to the server.
        """
        if self.server:
            if not force:
                # Send a "stop" command to the server and wait for it to terminate.
                self.command("stop")
                self.server.communicate()
                self.server = None
            else:
                # Forcibly kill the server process.
                self.server.kill()
                self.server = None

    def command(self, cmd: str):
        """
        Sends a command to the server process.

        :param cmd: The command to send to the server.
        :raises Exception: If the server is not running.
        """
        if self.server:
            # Write the command to the server's stdin and flush the buffer.
            self.server.stdin.write(f"{cmd}\n")
            self.server.stdin.flush()
        else:
            raise Exception("Server is not running")
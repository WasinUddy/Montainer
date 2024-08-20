import subprocess
import os
import boto3
import time

# S3 environment variables
S3_ENDPOINT = os.environ.get("S3_ENDPOINT")
S3_ACCESS_KEY = os.environ.get("S3_ACCESS_KEY")
S3_SECRET_KEY = os.environ.get("S3_SECRET_KEY")
S3_REGION = os.environ.get("S3_REGION")
S3_BUCKET = os.environ.get("S3_BUCKET")


class Server:
    def __init__(self, cwd: str):
        """
        Constructor for the Server class.

        :param cwd: The current working directory where the server will be started.
        """
        self.cwd = cwd
        self.server = None
        self.config_files = ("server.properties", "allowlist.json", "permissions.json")
        self.running = False

        # Check if the S3 environment variables are set
        if not S3_ENDPOINT or not S3_ACCESS_KEY or not S3_SECRET_KEY or not S3_REGION or not S3_BUCKET:
            self.s3_enabled = False
        else:
            self.s3_enabled = True
            self.s3 = boto3.client(
                "s3",
                endpoint_url=S3_ENDPOINT,
                aws_access_key_id=S3_ACCESS_KEY,
                aws_secret_access_key=S3_SECRET_KEY,
                region_name=S3_REGION
            )

            # Check if the S3 bucket exists, if not, create it.
            if not self.s3.head_bucket(Bucket=S3_BUCKET):
                self.s3.create_bucket(Bucket=S3_BUCKET)
            

        

    def start(self):
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

            # Set the running flag to True.
            self.running = True
                
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

        # Set the running flag to False.
        self.running = False
            

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
        
    def live_probe(self):
        """
        Check if the server is not in illegal state. to be used with Kubernetes liveness probe
        if the process died while flag is still True then the Server is in illegal state
        """
        
        # self.server.poll() is None if the process is still running
        if self.running!=(self.server.poll() is None):
            return False # Illegal state
        else:
            return True
        
    def backup(self):
        """
        Backup the server data to S3 bucket.
        """ 
        # Stop the server before backing up
        need_to_start = False
        if self.running:
            need_to_start = True
            self.stop()

        # Zip the server data worlds and configs
        current_time = time.time()
        os.system(f"zip -r /app/backup-{current_time}.zip /app/configs /app/minecraft_server/worlds /app/configs")

        # Upload the backup to S3 bucket
        self.s3.upload_file(f"/app/backup-{current_time}.zip", S3_BUCKET, f"backup-{current_time}.zip")

        # Remove the backup file
        os.remove(f"/app/backup-{current_time}.zip")

        # Start the server if it was running
        if need_to_start:
            self.start()


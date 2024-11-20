import os
import shutil
import subprocess
import boto3
import time

from settings import settings

class MinecraftServer:
    def __init__(self, cwd: str):
        """
        Initialize MinecraftServer class an abstraction for managing a Minecraft Bedrock Edition Server instance.

        Parameters:
        cwd (str): The working directory where the server instance is located.
        """
        self.cwd = cwd
        self.config_files = ('server.properties', 'allowlist.json', 'permissions.json')
        self.instance = None
        self.is_running = False
        self.log_file = None

        self.start() # Start the server instance on initialization

        if settings.AWS_S3_ENDPOINT:
            self.s3 = boto3.client(
                's3',
                endpoint_url=settings.AWS_S3_ENDPOINT,
                aws_access_key_id=settings.AWS_S3_KEY_ID,
                aws_secret_access_key=settings.AWS_S3_SECRET_KEY,
                region_name=settings.AWS_S3_REGION
            )

    def start(self):
        """
        Start the Minecraft server instance if it is not already running.
        """
        if not self.instance:

            # Sync config files from mapped volume to game instance directory
            for config_file in self.config_files:
                if not os.path.exists(f'./configs/{config_file}'):
                    shutil.copy(f'./instance/{config_file}', f'./configs/{config_file}')
                else:
                    shutil.copy(f'./configs/{config_file}', f'./instance/{config_file}')

            # Check if the log file exists and create it if it doesn't
            if not os.path.exists('instance.log'):
                open('instance.log', 'w').close()
            self.log_file = open('instance.log', 'r+')

            # Start the server instance
            self.instance = subprocess.Popen(
                ['./bedrock_server'],
                cwd=self.cwd,
                stdout=self.log_file,
                stderr=self.log_file,
                stdin=subprocess.PIPE,
                text=True,
                bufsize=1,
                universal_newlines=True
            )

            # Set the running flag to True
            self.is_running = True

    def stop(self, force_shutdown: bool = False):
        """
        Stop the Minecraft server instance if it is running.

        Parameters:
        force_shutdown (bool): Forcefully shutdown the server instance if True.
        """
        if self.instance:
            if not force_shutdown:
                self.instance.stdin.write('stop\n')
                self.instance.stdin.flush()
                self.instance = None
            else:
                self.instance.kill()
                self.instance = None

            self.is_running = False
            os.remove('instance.log')

    def send_command(self, command_string: str):
        """
        Send a command to the Minecraft server instance.

        Parameters:
        command_string (str): The command to send to the server instance.

        Raises:
        Exception: If the server instance is not running illegal state.
        """
        if self.instance:
            self.instance.stdin.write(f'{command_string}\n')
            self.instance.stdin.flush()

            if command_string == 'stop':
                self.instance = None
                self.is_running = False
                os.remove('instance.log')
        else:
            raise Exception('Server instance is not running.')

    def save_data(self):
        """
        Save the server persistent data to AWS S3 bucket.
        """
        self.stop()

        os.mkdir('./tmp')

        # Copy the world data to a temporary directory
        shutil.copytree('./instance/worlds', './tmp/worlds')

        # Copy the config files to a temporary directory
        for config_file in self.config_files:
            shutil.copy(f'./configs/{config_file}', f'./tmp/{config_file}')

        # Zip the temporary directory
        shutil.make_archive('./tmp', 'zip', './tmp')

        # Upload the zip file to AWS S3
        self.s3.upload_file(
            './tmp.zip',
            settings.AWS_S3_BUCKET_NAME,
            f'{settings.INSTANCE_NAME}_{int(time.time())}_backup.zip'
        )

        # Cleanup
        shutil.rmtree('./tmp')
        os.remove('./tmp.zip')

        self.start()
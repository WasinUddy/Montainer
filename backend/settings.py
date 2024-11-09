from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Subpath URL
    SUBPATH_URL: str = '/'

    # Backup S3 settings
    AWS_S3_ENDPOINT: str = ''
    AWS_S3_KEY_ID: str = ''
    AWS_S3_SECRET_KEY: str = ''
    AWS_S3_BUCKET_NAME: str = ''
    AWS_S3_REGION: str = ''

    class Config:
        env_file = '.env'
        env_file_encoding = 'utf-8'

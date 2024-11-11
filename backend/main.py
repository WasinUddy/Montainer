import os
from settings import settings

# Rewrite html with correct subpath URL
subpath = settings.SUBPATH_URL  # default is '/'
with open('./dist/index.html', 'r') as f:
    html = f.read()

if subpath not in html:
    html = html.replace("/assets/", f"{subpath}assets/")

os.remove('./dist/index.html')
with open('./dist/index.html', 'w') as f:
    f.write(html)

if __name__ == '__main__':
    import uvicorn
    from web_server import app

    uvicorn.run(app, host='0.0.0.0', port=8000, log_level='critical')
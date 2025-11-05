import os
import shutil

def copy_if_not_exists(src, dst):
    if os.path.exists(dst):
        return
    else:
        shutil.copy2(src, dst)
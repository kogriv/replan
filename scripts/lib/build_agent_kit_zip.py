"""Create a deterministic ZIP while preserving executable script bits."""

import os
import pathlib
import sys
import zipfile


root = pathlib.Path(sys.argv[1])
output = pathlib.Path(sys.argv[2])
with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
    for path in sorted((root, *root.rglob("*"))):
        relative = path.relative_to(root.parent).as_posix()
        if path.is_dir():
            relative += "/"
            data = b""
            mode = 0o40755
        else:
            data = path.read_bytes()
            mode = 0o100755 if os.access(path, os.X_OK) else 0o100644
        info = zipfile.ZipInfo(relative, (1980, 1, 1, 0, 0, 0))
        info.create_system = 3
        info.external_attr = mode << 16
        info.compress_type = zipfile.ZIP_DEFLATED
        archive.writestr(info, data)

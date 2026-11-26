#!/usr/bin/env python3
"""Verify the two distribution formats contain the native library reviewed for notices."""

import ctypes
import hashlib
import pathlib
import sys


# The reviewed Linux x64 asset in plugins/terminal/mygo-plugin.json.
# Keep this independent from the download manifest: changing the artifact
# must fail this release check until its notices are reviewed.
REVIEWED_SHA256 = "9dcd8af0de67778a561f436cb673a9fb1738be8fa00849dad49606baed1e57d7"


def flags(library_path):
    # See include/ghostty/vt/build_info.h at the pinned Ghostty source.
    library = ctypes.CDLL(str(library_path.resolve(strict=True)))
    get = library.ghostty_build_info
    get.argtypes = (ctypes.c_int, ctypes.c_void_p)
    get.restype = ctypes.c_int
    info = {}
    for key, name, kind in (
        (1, "simd", ctypes.c_bool),
        (2, "kitty_graphics", ctypes.c_bool),
        (4, "optimize", ctypes.c_int),
    ):
        out = kind()
        status = get(key, ctypes.byref(out))
        if status != 0:
            raise ValueError(f"Ghostty build info {name} failed: {status}")
        info[name] = out.value
    return info


def verify(deb_library, tar_library):
    for kind, path in (("DEB", deb_library), ("tar", tar_library)):
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if digest != REVIEWED_SHA256:
            raise ValueError(f"{kind} Ghostty SHA-256 changed: {digest}; review native notices")
    info = flags(deb_library)
    print(f"Ghostty {REVIEWED_SHA256}: {info}")
    if info != {"simd": True, "kitty_graphics": True, "optimize": 3}:
        raise ValueError("Unexpected Ghostty build configuration; review native notices")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(f"Usage: {sys.argv[0]} DEB_LIBRARY TAR_LIBRARY")
    try:
        verify(*(pathlib.Path(p) for p in sys.argv[1:]))
    except (ValueError, OSError, AttributeError) as e:
        sys.exit(str(e))

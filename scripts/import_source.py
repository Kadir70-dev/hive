#!/usr/bin/env python3
"""Import the validated Hive Society source archive into the repository."""
from pathlib import Path, PurePosixPath
import shutil
from zipfile import ZipFile

root = Path(__file__).resolve().parents[1]
archive = root / "import" / "hive-source.zip"
if not archive.is_file():
    raise SystemExit("Source ZIP missing; no import performed")
count = 0
with ZipFile(archive) as source:
    for entry in source.infolist():
        if entry.is_dir():
            continue
        parts = PurePosixPath(entry.filename).parts
        if len(parts) < 2 or parts[0] != "hive-society-nextjs":
            raise SystemExit(f"Unexpected ZIP path: {entry.filename}")
        relative = PurePosixPath(*parts[1:])
        if ".." in relative.parts or any(p == ".git" for p in relative.parts):
            raise SystemExit(f"Unsafe ZIP path: {entry.filename}")
        dest = root.joinpath(*relative.parts).resolve()
        if not dest.is_relative_to(root):
            raise SystemExit(f"Escaping destination: {entry.filename}")
        dest.parent.mkdir(parents=True, exist_ok=True)
        with source.open(entry) as s, dest.open("wb") as d:
            shutil.copyfileobj(s, d)
        count += 1
print(f"Imported {count} source files from the verified ZIP")

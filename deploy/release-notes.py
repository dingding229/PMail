"""Print the exact release section, failing if the tagged notes are missing."""
import re
import sys
from pathlib import Path

version = sys.argv[1]
markdown = (Path(__file__).resolve().parents[1] / "CHANGELOG.md").read_text(encoding="utf-8")
sections = re.split(r"^## ", markdown, flags=re.MULTILINE)[1:]
for section in sections:
    title, _, notes = section.partition("\n")
    if re.match(rf"(?:\[{re.escape(version)}\]|{re.escape(version)})(?:\s|$)", title) and notes.strip():
        print(notes.strip())
        break
else:
    sys.exit(f"No changelog section for {version}")

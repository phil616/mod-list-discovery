#!/usr/bin/env python3
"""Check repository-relative Markdown links without network access."""
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
errors = []
files = [ROOT / 'README.md', *sorted((ROOT / 'docs').rglob('*.md'))]
for source in files:
    text = re.sub(r'```.*?```', '', source.read_text(encoding='utf-8'), flags=re.S)
    for match in re.finditer(r'!?\[[^\]]*\]\(([^)]+)\)', text):
        target = match[1].split(' "')[0].strip('<>')
        url = urlsplit(target)
        if url.scheme or url.netloc or not url.path:
            continue
        destination = (source.parent / unquote(url.path)).resolve()
        if not destination.is_relative_to(ROOT) or not destination.exists():
            errors.append(f'{source.relative_to(ROOT)}: missing local link {target}')
        if destination.suffix.lower() in {'.png', '.jpg', '.jpeg', '.gif', '.webp'}:
            errors.append(f'{source.relative_to(ROOT)}: debug image exposed: {target}')
if errors:
    sys.exit('\n'.join(errors))
print(f'Documentation links OK ({len(files)} Markdown files; debug screenshots not exposed)')

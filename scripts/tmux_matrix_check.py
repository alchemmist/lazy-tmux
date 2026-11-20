"""Compare complete matrix transcripts; exit 1 for regression, 2 for bad data."""
import json
import re
import sys
from pathlib import Path


def expected(releases):
    versions = set()
    for release in releases:
        tag = release['tag_name']
        match = re.fullmatch(r'(\d+)\.(\d+)[a-z]*', tag)
        if match and tuple(map(int, match.groups())) >= (2, 9):
            versions.add(tag)
    if not versions:
        raise ValueError('empty release list')
    return versions


def passed(text, versions):
    rows = {}
    for line in text.splitlines():
        if line.startswith('  tmux '):
            _, version, result = line.split()
            if version in rows or result not in {'✓', '✗'}:
                raise ValueError(f'invalid matrix row: {line}')
            rows[version] = result
    if rows.keys() != versions:
        raise ValueError(f'incomplete matrix: {sorted(rows.keys() ^ versions)}')
    return {v for v in rows if rows[v] == '✓'}


def compare(base, head, versions):
    supported = passed(base, versions)
    if not supported:
        raise ValueError('base has no passing version')
    return sorted(supported - passed(head, versions))


if __name__ == '__main__':
    try:
        base, head, releases = map(Path, sys.argv[1:])
        missing = compare(base.read_text(), head.read_text(), expected(json.loads(releases.read_text())))
    except (ValueError, KeyError, TypeError, OSError) as err:
        print(f'Matrix infrastructure failure: {err}')
        sys.exit(2)
    if missing:
        print('Previously supported versions lost: ' + ', '.join(missing))
        sys.exit(1)

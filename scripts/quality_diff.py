"""PR guardrails. Run coverage with Python 3; exclusions also needs PyYAML."""
import argparse
import ast
import collections
import json
import os
from pathlib import Path
import re
import subprocess


def git(*args):
    return subprocess.check_output(['git', *args], text=True)


def added(diff):
    path = None
    for row in diff.splitlines():
        if row.startswith('diff --git '):
            path = None
        elif row.startswith('+++ ') and path is None:
            raw = row[4:].rstrip('\t')
            if raw.startswith('"'):
                raw = ast.literal_eval(raw)
            if raw.startswith('b/'):
                path = raw[2:]
        elif row.startswith('@@ '):
            line = int(re.search(r' \+(\d+)', row)[1])
        elif path and row.startswith('+'):
            yield path, line, row[1:]
            line += 1


def exclusions(config):
    import yaml
    data = yaml.safe_load(config) or {}
    items = []
    for group in ('linters', 'formatters'):
        for key, value in (data.get(group, {}).get('exclusions') or {}).items():
            for item in value if isinstance(value, list) else [value]:
                items.append((group, key, json.dumps(item, sort_keys=True)))
    return collections.Counter(items)


def coverage(changes, profile, module):
    lines = {(p, n) for p, n, _ in changes if not p.endswith('_test.go')}
    states = {}
    blocks = {}
    for row in profile.splitlines()[1:]:
        name, start, end, statements, count = re.fullmatch(r'(.+):(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)', row).groups()
        key = (name, start, end, statements, row.rsplit(' ', 2)[0])
        blocks[key] = blocks.get(key, 0) + int(count)
    for (name, start, end, statements, _), count in blocks.items():
        path = name.removeprefix(module + '/')
        if int(statements):
            for n in range(int(start), int(end) + 1):
                if (path, n) in lines:
                    states[path, n] = states.get((path, n), True) and int(count) > 0
    missed = [f'{p}:{n}' for (p, n), hit in sorted(states.items()) if not hit]
    if not states:
        return 'No changed lines intersect Go coverage blocks in this profile.'
    return f'Changed Go lines intersecting coverage blocks: {sum(states.values())}/{len(states)} covered.\n' + '\n'.join(missed)


def invalid_directive(text):
    match = re.match(r'//\s*nolint\b', text)
    if not match:
        return False
    directive = re.fullmatch(r'//nolint:([\w-]+(?:,[\w-]+)*)\s+//\s*(\S.*)', text[match.start():])
    return not directive or 'all' in directive[1].split(',')


def comments(source):
    # Consume complete Go literals and block comments before looking for line
    # comments, including raw strings spanning multiple changed lines.
    token = re.compile(r'`[^`]*`|"(?:\\.|[^"\\])*"|\'(?:\\.|[^\'\\])*\'|/\*[\s\S]*?\*/|//[^\n]*')
    line, end = 1, 0
    for match in token.finditer(source):
        line += source.count('\n', end, match.start())
        if match[0].startswith('//'):
            yield line, match[0]
        line += match[0].count('\n')
        end = match.end()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['coverage', 'suppressions'])
    parser.add_argument('--base', required=True)
    args = parser.parse_args()
    base = git('merge-base', args.base, 'HEAD').strip()
    changes = list(added(git('-c', 'core.quotePath=false', 'diff', '--no-ext-diff', '--no-color', '-U0', base, '--', '*.go')))
    if args.mode == 'coverage':
        report = coverage(changes, Path('cover.out').read_text(), subprocess.check_output(['go', 'list', '-m'], text=True).strip())
        print(report)
        if summary := os.environ.get('GITHUB_STEP_SUMMARY'):
            with open(summary, 'a') as out:
                print('```text', report, '```', sep='\n', file=out)
        return 0
    errors = []
    for path in sorted({p for p, _, _ in changes}):
        touched = {line for p, line, _ in changes if p == path}
        for line, comment in comments(Path(path).read_text()):
            if line in touched and invalid_directive(comment):
                errors.append(f'{path}:{line}: name specific linters and explain the exception')
    previous = git('show', base + ':.golangci.yml') if git('ls-tree', base, '--', '.golangci.yml').strip() else ''
    for item in exclusions(Path('.golangci.yml').read_text()) - exclusions(previous):
        errors.append(f'New or modified exclusion requires policy review: {item}')
    for error in errors:
        print(error)
    return bool(errors)


if __name__ == '__main__':
    raise SystemExit(main())

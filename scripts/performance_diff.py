"""Compare interleaved, paired Go benchmark samples on one runner."""
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from statistics import median


CASES = {'BenchmarkSpinnerTickLargePicker', 'BenchmarkProcessSnapshotForegroundCommands'}


def sample(root):
    out = subprocess.check_output(['go', 'test', '-run=^$', '-bench=^(' + '|'.join(sorted(CASES)) + ')$',
                                   '-cpu=1', '-benchtime=350ms', '-count=1', '-benchmem', '-p=1',
                                   './internal/picker', './internal/tmux'], cwd=root, text=True)
    rows = {}
    for name, ns in re.findall(
        r'^(Benchmark\w+)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op', out, re.M):
        if name in rows or float(ns) <= 0:
            raise ValueError(f'duplicate or nonpositive result for {name}')
        rows[name] = float(ns)
    if set(rows) != CASES:
        raise ValueError('missing benchmark results: ' + out)
    return rows


def regression(ratios):
    return median(ratios) > 1.25 and sum(r > 1.15 for r in ratios) >= 4


def main(base):
    root = Path.cwd()
    sha = subprocess.check_output(['git', 'merge-base', base, 'HEAD'], text=True).strip()
    with tempfile.TemporaryDirectory() as tmp:
        old = Path(tmp) / 'base'
        subprocess.run(['git', 'worktree', 'add', '--quiet', '--detach', str(old), sha], check=True)
        try:
            # Warm compile caches and alternate order to reduce thermal bias.
            sample(old); sample(root)
            ratios = {k: [] for k in CASES}
            values = {k: ([], []) for k in CASES}
            for i in range(5):
                order = (root, old) if i % 2 else (old, root)
                pairs = {p: sample(p) for p in order}
                for key in CASES:
                    ratios[key].append(pairs[root][key] / pairs[old][key])
                    for dest, p in zip(values[key], (old, root)):
                        dest.append(pairs[p][key])
            report = ['| Workload | Base ns/op (median) | Head ns/op (median) | Paired change |',
                      '|---|---:|---:|---:|']
            for key in sorted(CASES):
                before, after = map(median, values[key])
                report.append(f'| {key} | {before:g} | {after:g} | {median(ratios[key])-1:+.1%} |')
            print('\n'.join(report))
            if path := os.getenv('GITHUB_STEP_SUMMARY'):
                with open(path, 'a') as stream:
                    print('\n'.join(report), file=stream)
            return any(map(regression, ratios.values()))
        finally:
            subprocess.run(['git', 'worktree', 'remove', '--force', str(old)], check=True)


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

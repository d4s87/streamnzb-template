#!/usr/bin/env python3

from pathlib import Path
import re
import sys

WORKFLOWS = Path('.github/workflows')
PRIVILEGED_TRIGGERS = ('pull_request_target:', 'workflow_run:')
FLOATING_ACTION_REFS = {'main', 'master', 'latest', 'develop', 'development'}


def audit(path: Path) -> list[str]:
    text = path.read_text(encoding='utf-8')
    errors: list[str] = []

    if not re.search(r'^permissions:\s*(?:$|\n)', text, flags=re.MULTILINE):
        errors.append('missing explicit top-level permissions block')

    if re.search(r'^permissions:\s*write-all\s*$', text, flags=re.MULTILINE):
        errors.append('uses permissions: write-all')

    privileged = any(
        re.search(rf'^\s{{2}}{re.escape(trigger)}\s*$', text, flags=re.MULTILINE)
        for trigger in PRIVILEGED_TRIGGERS
    )
    checks_out = bool(re.search(r'uses:\s*actions/checkout@', text))
    if privileged and checks_out:
        errors.append(
            'privileged pull_request_target/workflow_run workflow checks out repository code'
        )

    if re.search(r'\bsecrets:\s*inherit\b', text):
        errors.append('uses secrets: inherit')

    for match in re.finditer(r'^\s*-?\s*uses:\s*([^\s#]+)', text, flags=re.MULTILINE):
        action = match.group(1)
        if action.startswith('./') or action.startswith('docker://'):
            continue
        if '@' not in action:
            errors.append(f'action reference is not versioned: {action}')
            continue
        _, ref = action.rsplit('@', 1)
        if ref.lower() in FLOATING_ACTION_REFS:
            errors.append(f'action uses floating ref @{ref}: {action}')

    return errors


def main() -> int:
    failures: list[tuple[Path, str]] = []
    for path in sorted(WORKFLOWS.glob('*.y*ml')):
        for error in audit(path):
            failures.append((path, error))

    if failures:
        print('Workflow security audit failed:', file=sys.stderr)
        for path, error in failures:
            print(f'  {path}: {error}', file=sys.stderr)
        return 1

    print('Workflow security audit passed.')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

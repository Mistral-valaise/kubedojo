#!/usr/bin/env bash
# A pipeline definition whose stages are in the wrong order, with one obsolete
# stage and one line that has to be copied from one stage to another. The last
# stage is already in place: moving the last paragraph of a file is the one
# awkward case of "dap", and the task does not need it.
set -euo pipefail

base=/opt/course/vim-reorder-stages
rm -rf "$base"
mkdir -p "$base/.grade"

cat > "$base/pipeline.conf" <<'EOF'
# Release pipeline of the checkout service.
# Stages run in the order in which they appear in this file.

[package]
image = docker:27
script = make image push
timeout = 600

[test]
image = golang:1.24
script = make test
timeout = 600
on_failure = notify-team

[legacy-upload]
image = ftp-client:0.9
script = ./ci/upload-ftp.sh
timeout = 120

[build]
image = golang:1.24
script = make build
timeout = 600

[lint]
image = golangci-lint:1.62
script = make lint
timeout = 300

[deploy]
image = deployer:3.2
script = ./ci/deploy.sh production
timeout = 900
EOF

cp "$base/pipeline.conf" "$base/.grade/original"

cat > "$base/.grade/check.py" <<'PY'
#!/usr/bin/env python3
"""Grader of vim-reorder-stages: reads the stages of pipeline.conf and tests one property."""
import re
import sys

BASE = '/opt/course/vim-reorder-stages'
WANT = ['lint', 'build', 'test', 'package', 'deploy']
COPIED = 'on_failure = notify-team'


def fail(msg):
    print(msg, file=sys.stderr)
    sys.exit(1)


def read(path):
    """Returns (lines before the first stage, [(stage name, [settings])])."""
    head, stages = [], []
    try:
        text = open(path, encoding='utf-8', errors='replace').read()
    except OSError:
        fail(f'{path} does not exist')
    for raw in text.split('\n'):
        line = re.sub(r'\s*=\s*', ' = ', raw.strip(), count=1)
        if not line:
            continue
        if line.startswith('[') and line.endswith(']'):
            stages.append((line[1:-1].strip(), []))
        elif stages:
            stages[-1][1].append(line)
        else:
            head.append(line)
    return head, stages


def main():
    what = sys.argv[1]
    head, stages = read(f'{BASE}/pipeline.conf')
    names = [n for n, _ in stages]
    if what == 'order':
        got = [n for n in names if n in WANT]
        if got != WANT:
            fail('the stages appear as: ' + ', '.join(names) + '; wanted: ' + ', '.join(WANT))
    elif what == 'legacy-gone':
        if 'legacy-upload' in names:
            fail('the stage [legacy-upload] is still there')
        left = [l for _, body in stages for l in body if 'ftp' in l]
        if left:
            fail('lines of the removed stage are left in another stage: ' + '; '.join(left))
    elif what == 'on-failure':
        bodies = [body for n, body in stages if n == 'deploy']
        if len(bodies) != 1:
            fail(f'there must be exactly one [deploy] stage, found {len(bodies)}')
        n = bodies[0].count(COPIED)
        if n != 1:
            fail(f'[deploy] contains the line "{COPIED}" {n} times, wanted once')
    elif what == 'intact':
        ohead, ostages = read(f'{BASE}/.grade/original')
        if head != ohead:
            fail('the comment lines at the top of the file changed or moved')
        extra = [n for n in names if n not in WANT and n != 'legacy-upload']
        if extra:
            fail('unknown stage(s): ' + ', '.join(extra))
        for name, obody in ostages:
            if name not in WANT:
                continue
            found = [body for n, body in stages if n == name]
            if len(found) != 1:
                fail(f'there must be exactly one [{name}] stage, found {len(found)}')
            body = found[0]
            if name == 'deploy':
                body = [l for l in body if l != COPIED]
            if body != obody:
                fail(f'the settings of [{name}] are not the original ones: ' + '; '.join(body))
    else:
        fail(f'unknown check {what}')
    print('ok')


main()
PY

chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/pipeline.conf"

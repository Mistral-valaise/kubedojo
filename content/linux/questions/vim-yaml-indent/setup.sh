#!/usr/bin/env bash
# A Pod manifest into which a block was pasted with tab characters and at the
# wrong depth, and whose last block hangs one level too deep. The grader in
# .grade/check.py parses the result; there is no YAML library in the image.
set -euo pipefail

base=/opt/course/vim-yaml-indent
rm -rf "$base"
mkdir -p "$base/.grade"

T=$'\t'
cat > "$base/pod.yaml" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: api
  namespace: shop
  labels:
    app: api
spec:
  containers:
    - name: api
      image: registry.example.com/shop/api:2.4.1
      ports:
        - containerPort: 8080
${T}env:
${T}${T}- name: LOG_LEVEL
${T}${T}  value: info
${T}${T}- name: DB_HOST
${T}${T}  value: postgres.shop.svc
${T}${T}- name: DB_PORT
${T}${T}  value: "5432"
${T}resources:
${T}${T}requests:
${T}${T}${T}cpu: 100m
${T}${T}${T}memory: 128Mi
      volumeMounts:
        - name: config
          mountPath: /etc/api
      volumes:
        - name: config
          configMap:
            name: api-config
EOF

cat > "$base/.grade/check.py" <<'PY'
#!/usr/bin/env python3
"""Grader of vim-yaml-indent: parses pod.yaml and tests one property of its structure."""
import copy
import re
import sys


class Bad(Exception):
    """The file is not YAML of the kind this grader reads."""


def fail(msg):
    print(msg, file=sys.stderr)
    sys.exit(1)


def strip_comment(text):
    quote = None
    for i, c in enumerate(text):
        if quote:
            if c == quote:
                quote = None
        elif c in '"\'':
            quote = c
        elif c == '#' and (i == 0 or text[i - 1] in ' \t'):
            return text[:i].rstrip()
    return text.rstrip()


def plain(text):
    text = text.strip()
    if text in ('', '~', 'null'):
        return None
    if re.fullmatch(r'-?\d+', text):
        return int(text)
    if text in ('true', 'false'):
        return text == 'true'
    return text


def flow(text, n):
    """Parses a flow collection: [a, b] or {a: 1, b: {c: 2}}."""
    pos = 0

    def bad():
        raise Bad(f'line {n}: cannot read "{text}"')

    def skip():
        nonlocal pos
        while pos < len(text) and text[pos] == ' ':
            pos += 1

    def token(stops):
        nonlocal pos
        skip()
        if pos < len(text) and text[pos] in '"\'':
            end = text.find(text[pos], pos + 1)
            if end < 0:
                bad()
            word = text[pos + 1:end]
            pos = end + 1
            return word
        start = pos
        while pos < len(text) and text[pos] not in stops:
            pos += 1
        return plain(text[start:pos])

    def close(end):
        """After an item: a comma, or the closing bracket (left for the caller)."""
        nonlocal pos
        skip()
        if pos < len(text) and text[pos] == ',':
            pos += 1
        elif not (pos < len(text) and text[pos] == end):
            bad()

    def value():
        nonlocal pos
        skip()
        if pos >= len(text):
            bad()
        if text[pos] == '[':
            pos += 1
            items = []
            while True:
                skip()
                if pos >= len(text):
                    bad()
                if text[pos] == ']':
                    pos += 1
                    return items
                items.append(value())
                close(']')
        if text[pos] == '{':
            pos += 1
            items = {}
            while True:
                skip()
                if pos >= len(text):
                    bad()
                if text[pos] == '}':
                    pos += 1
                    return items
                key = token(':,}')
                skip()
                if pos >= len(text) or text[pos] != ':':
                    bad()
                pos += 1
                items[str(key)] = value()
                close('}')
        return token(',]}')

    result = value()
    skip()
    if pos != len(text):
        bad()
    return result


def scalar(text, n):
    text = text.strip()
    if text[:1] in ('"', "'"):
        if len(text) < 2 or text[-1] != text[0]:
            raise Bad(f'line {n}: a quote is not closed')
        return text[1:-1]
    if text[:1] in ('[', '{'):
        return flow(text, n)
    if text[:1] in ('|', '>'):
        raise Bad(f'line {n}: block scalars (| and >) are not expected in this file')
    return plain(text)


def pair(text):
    """Splits "key: value"; returns None for anything else."""
    if text[0] in '"\'':
        end = text.find(text[0], 1)
        if end < 0:
            return None
        m = re.match(r'^\s*:(?:\s+(.*))?$', text[end + 1:])
        return (text[1:end], (m.group(1) or '').strip()) if m else None
    if text[0] in '[{':
        return None
    m = re.match(r'^(.+?)\s*:(?:\s+(.*))?$', text)
    return (m.group(1), (m.group(2) or '').strip()) if m else None


def dash(text):
    return text == '-' or text.startswith('- ')


def load(path):
    """Parses block-style YAML: mappings, lists, scalars, flow collections and comments."""
    try:
        text = open(path, encoding='utf-8', errors='replace').read()
    except OSError:
        fail(f'{path} does not exist')
    rows = []
    for n, raw in enumerate(text.split('\n'), 1):
        raw = raw.rstrip('\r')
        lead = raw[:len(raw) - len(raw.lstrip(' \t'))]
        body = strip_comment(raw.strip())
        if not body or body == '---':
            continue
        if '\t' in lead:
            raise Bad(f'line {n}: tab character in the indentation')
        rows.append([len(lead), body, n])
    if not rows:
        raise Bad('the file is empty')
    pos = 0

    def deeper(ind):
        if pos < len(rows) and rows[pos][0] > ind:
            raise Bad(f'line {rows[pos][2]}: the indentation matches no enclosing level')

    def block(ind):
        return sequence(ind) if dash(rows[pos][1]) else mapping(ind)

    def child(ind):
        if pos < len(rows) and rows[pos][0] > ind:
            return block(rows[pos][0])
        return None

    def sequence(ind):
        nonlocal pos
        items = []
        while pos < len(rows) and rows[pos][0] == ind and dash(rows[pos][1]):
            text, n = rows[pos][1], rows[pos][2]
            rest = text[1:].lstrip(' ')
            if not rest:
                pos += 1
                items.append(child(ind))
            elif pair(rest):
                inner = ind + len(text) - len(rest)
                rows[pos][0], rows[pos][1] = inner, rest
                items.append(mapping(inner))
            else:
                pos += 1
                items.append(scalar(rest, n))
        deeper(ind)
        return items

    def mapping(ind):
        nonlocal pos
        items = {}
        while pos < len(rows) and rows[pos][0] == ind and not dash(rows[pos][1]):
            kv, n = pair(rows[pos][1]), rows[pos][2]
            if not kv:
                raise Bad(f'line {n}: expected "key: value"')
            key, value = kv
            if key in items:
                raise Bad(f'line {n}: the key "{key}" appears twice in one mapping')
            pos += 1
            if value:
                items[key] = scalar(value, n)
            elif pos < len(rows) and rows[pos][0] == ind and dash(rows[pos][1]):
                items[key] = sequence(ind)
            else:
                items[key] = child(ind)
        deeper(ind)
        return items

    doc = block(rows[0][0])
    if pos < len(rows):
        raise Bad(f'line {rows[pos][2]}: the indentation matches no enclosing level')
    return doc


def dig(doc, *path):
    """doc[path...] or None when a step is missing or of the wrong type."""
    for step in path:
        if isinstance(step, int):
            if not isinstance(doc, list) or len(doc) <= step:
                return None
        elif not isinstance(doc, dict) or step not in doc:
            return None
        doc = doc[step]
    return doc


def manifest(path):
    try:
        doc = load(path)
    except Bad as e:
        fail(f'{path} is not valid YAML: {e}')
    if not isinstance(doc, dict):
        fail(f'{path} must be a mapping at the top level')
    return doc


FILE = '/opt/course/vim-yaml-indent/pod.yaml'
ENV = [
    {'name': 'LOG_LEVEL', 'value': 'info'},
    {'name': 'DB_HOST', 'value': 'postgres.shop.svc'},
    {'name': 'DB_PORT', 'value': '5432'},
]
RESOURCES = {'requests': {'cpu': '100m', 'memory': '128Mi'}}
MOUNTS = [{'name': 'config', 'mountPath': '/etc/api'}]
VOLUMES = [{'name': 'config', 'configMap': {'name': 'api-config'}}]
METADATA = {'name': 'api', 'namespace': 'shop', 'labels': {'app': 'api'}}


def owners(doc, key):
    """Where a key was found, as a list of paths: shows a block at the wrong depth."""
    found = []

    def walk(node, path):
        if isinstance(node, dict):
            for k, v in node.items():
                if k == key:
                    found.append(path or '(top level)')
                walk(v, f'{path}.{k}' if path else k)
        elif isinstance(node, list):
            for i, v in enumerate(node):
                walk(v, f'{path}[{i}]')

    walk(doc, '')
    return found


def placed(doc, key, home, want):
    where = owners(doc, key)
    if where != [home]:
        fail(f'"{key}" must be a key of {home} and nowhere else; found in: ' + (', '.join(where) or 'nowhere'))
    spec = dig(doc, 'spec')
    got = dig(spec, 'containers', 0, key) if home.endswith(']') else dig(spec, key)
    if got != want:
        fail(f'{home}.{key} does not have the expected content; its entries are probably nested at the wrong depth')


def main():
    what = sys.argv[1]
    doc = manifest(FILE)
    if what == 'env':
        placed(doc, 'env', 'spec.containers[0]', ENV)
    elif what == 'resources':
        placed(doc, 'resources', 'spec.containers[0]', RESOURCES)
    elif what == 'volumes':
        placed(doc, 'volumes', 'spec', VOLUMES)
        placed(doc, 'volumeMounts', 'spec.containers[0]', MOUNTS)
    elif what == 'unchanged':
        containers = dig(doc, 'spec', 'containers')
        if not isinstance(containers, list) or len(containers) != 1:
            fail('spec.containers must be a list with exactly one container')
        want = {
            'apiVersion': 'v1', 'kind': 'Pod', 'metadata': METADATA,
            'name': 'api', 'image': 'registry.example.com/shop/api:2.4.1', 'ports': [{'containerPort': 8080}],
        }
        got = {k: dig(doc, k) for k in ('apiVersion', 'kind', 'metadata')}
        got.update({k: dig(containers[0], k) for k in ('name', 'image', 'ports')})
        wrong = [k for k in want if got[k] != want[k]]
        if wrong:
            fail('changed, lost or moved: ' + ', '.join(wrong))
    else:
        fail(f'unknown check {what}')
    print('ok')


main()
PY

chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/pod.yaml"

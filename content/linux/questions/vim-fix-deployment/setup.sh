#!/usr/bin/env bash
# A Deployment manifest with three defects: the wrong number of replicas, a
# mistyped image, and a container without port and resources. The grader in
# .grade/check.py parses the result; there is no YAML library in the image.
set -euo pipefail

base=/opt/course/vim-fix-deployment
rm -rf "$base"
mkdir -p "$base/.grade"

cat > "$base/deployment.yaml" <<'EOF'
# Deployment of the shop frontend. Review before applying.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: shop
  labels:
    app: web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: ngnix:1.27-alpine
          env:
            - name: BACKEND_URL
              value: http://api.shop.svc:8080
          volumeMounts:
            - name: cache
              mountPath: /var/cache/nginx
        - name: log-shipper
          image: busybox:1.36
          args: ["sh", "-c", "tail -F /var/log/nginx/access.log"]
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
      volumes:
        - name: cache
          emptyDir: {}
EOF
cp "$base/deployment.yaml" "$base/.grade/original.yaml"

cat > "$base/.grade/check.py" <<'PY'
#!/usr/bin/env python3
"""Grader of vim-fix-deployment: parses deployment.yaml and tests one property."""
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


BASE = '/opt/course/vim-fix-deployment'
CONTAINERS = ('spec', 'template', 'spec', 'containers')


def web(doc):
    containers = dig(doc, *CONTAINERS)
    if not isinstance(containers, list):
        fail('spec.template.spec.containers is not a list any more')
    found = [c for c in containers if isinstance(c, dict) and c.get('name') == 'web']
    if len(found) != 1:
        fail(f'expected exactly one container named "web", found {len(found)}')
    return found[0]


def number(value):
    return isinstance(value, int) and not isinstance(value, bool)


def skeleton(doc):
    """The manifest without the four things the task changes."""
    doc = copy.deepcopy(doc)
    if isinstance(dig(doc, 'spec'), dict):
        doc['spec'].pop('replicas', None)
    for c in dig(doc, *CONTAINERS) or []:
        if isinstance(c, dict) and c.get('name') == 'web':
            for key in ('image', 'ports', 'resources'):
                c.pop(key, None)
    return doc


def main():
    what = sys.argv[1]
    doc = manifest(f'{BASE}/deployment.yaml')
    if what == 'replicas':
        got = dig(doc, 'spec', 'replicas')
        if not number(got) or got != 3:
            fail(f'spec.replicas is {got!r}, wanted the number 3')
    elif what == 'image':
        got = web(doc).get('image')
        if got != 'nginx:1.27-alpine':
            fail(f'the image of container web is {got!r}')
    elif what == 'port':
        ports = web(doc).get('ports')
        if not isinstance(ports, list):
            fail('container web has no list "ports"; is it a key of the container, in the column of "image"?')
        if not any(isinstance(p, dict) and number(p.get('containerPort')) and p['containerPort'] == 8080 for p in ports):
            fail('no entry of ports has containerPort: 8080 (a number, not a quoted string)')
    elif what == 'resources':
        got = dig(web(doc), 'resources', 'requests')
        if got != {'cpu': '100m', 'memory': '64Mi'}:
            fail(f'resources.requests of container web is {got!r}, wanted cpu 100m and memory 64Mi')
    elif what == 'rest':
        try:
            original = load(f'{BASE}/.grade/original.yaml')
        except Bad as e:
            fail(f'the reference copy is broken, reset the question: {e}')
        if skeleton(doc) != skeleton(original):
            fail('something besides replicas, the image, the ports and the resources of container web changed')
    else:
        fail(f'unknown check {what}')
    print('ok')


main()
PY

chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/deployment.yaml"

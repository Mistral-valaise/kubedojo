# Networking & SSH

**8% of the skills assessment.** "It does not connect" has three possible layers: the name, the
port, the application. Ask them in that order, and learn to move files and commands over ssh.

## The 20% that solves 80%

### Name, port, status: three questions, three tools

```bash
getent hosts db.internal                 # 1. does the name resolve? (as applications do: /etc/hosts, then DNS)
dig +short db.internal                   #    DNS only; empty output = no record
nc -zv db.internal 5432                  # 2. does the port accept a connection? (-w 3: give up after 3s)
curl -s -o /dev/null -w '%{http_code}\n' -m 5 http://web:8080/health   # 3. what does HTTP answer?
curl -i http://web:8080/health           #    status line, headers and body
```

| What you see | Layer | Meaning |
|---|---|---|
| `Could not resolve host` (curl exit 6) | DNS | the name is wrong or the resolver is; no packet was sent to any server |
| `Connection refused` (curl exit 7) | TCP | the host answered: no process listens on that address and port |
| hangs, then `timed out` (curl exit 28) | TCP | no answer at all: firewall, wrong IP, host down |
| `HTTP 404`, `503`, ... (curl exit 0) | HTTP | network fine; the application says no |

Stop at the first layer that fails. An open port is not a healthy service: only the status code
says what the application thinks. `curl -f` turns a 4xx/5xx into exit code 22 for scripts.

### Who listens, and on which address

```bash
ss -ltn                                  # listening TCP sockets, numeric
sudo ss -ltnp                            # ... with the process (name and pid)
ss -ltn 'sport = :8080'                  # only this port
ip -br addr                              # the addresses of this host, one line per interface
ip route                                 # where packets leave; "default via" is the gateway
```

Read the **Local Address:Port** column:

- `127.0.0.1:8080` loopback: reachable from this machine only. From anywhere else: refused.
- `0.0.0.0:8080` (or `*:8080`) every IPv4 address of the host.
- `10.0.0.5:8080` only through that one address.

"Works with `curl localhost`, refused from another machine" is a loopback bind. The address is
a setting of the service (`listen`, `bind`, `--bind`): change it there, restart, look at `ss` again.
Services read their configuration when they start; an edited file changes nothing by itself.

### ssh: a shell, or one command

```bash
ssh web1                                 # a shell on web1
ssh web1 'df -h /var; uptime'            # run there, output here; quote to keep it in one piece
ssh web1 'sha256sum app.tar.gz' > remote.sha256     # the redirection is outside: the file is written HERE
ssh -i ~/.ssh/deploy_ed25519 -p 2222 deploy@web1    # another key, port, user
ssh -v web1                              # why does the login fail? every step, one per line
```

`Permission denied (publickey)` is the last line of several. The usual cause is on your side:

```text
WARNING: UNPROTECTED PRIVATE KEY FILE!   Permissions 0644 for 'deploy_ed25519' are too open.
```

ssh ignores a private key that others can read: `chmod 600 key`, and `chmod 700 ~/.ssh`. Options
you type every time belong in `~/.ssh/config`:

```text
Host web1
  HostName web1.internal
  User deploy
  IdentityFile ~/.ssh/deploy_ed25519
```

### Copying files over ssh

```bash
scp -p app.tar.gz web1:releases/         # push; -p keeps mode and times
scp -p web1:/var/log/app.log .           # pull: the side with "host:" is the remote one
scp -rp conf/ web1:backup/               # a directory
rsync -a conf/ web1:backup/conf/         # only what changed; -a = recursive + modes, times, links
rsync -a -e 'ssh -i key' src/ user@host:dst/      # rsync over ssh with options
```

- A relative remote path starts in the remote home directory; `web1:` alone is the home itself.
- rsync: a trailing slash on the source means "the contents of", without it the directory itself.
- Verify a transfer: `sha256sum file` on both ends, or `sha256sum -c list` against a saved list.

## Habits that save points

- Reproduce from where the client is. A test on the server itself passes for a loopback-only
  service and tells you nothing about the network.
- Refused = nothing listens there (look at `ss -ltn` on the server). Timeout = something drops
  the packets. Different faults, different fixes.
- Give every probe a time limit (`curl -m 5`, `nc -w 3`), or a blocked port costs you minutes.
- After changing a service: restart, then check `ss` and the client's request, not the file.
- `curl -s` hides errors as well as the progress bar: use `-sS` to keep the error message.
- In a container, `/etc/hosts` and `/etc/resolv.conf` are mounted files: edit them in place
  (`vim`, `>>`); `sed -i` fails on them because it replaces the file.

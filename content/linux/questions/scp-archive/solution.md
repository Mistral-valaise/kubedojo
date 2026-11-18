**1. Read what ssh says.** "Permission denied (publickey)" is only the last line:

```bash
ssh {{host}}
cd /opt/course/scp-archive
ssh -i keys/archive_ed25519 scp-archive@localhost
# WARNING: UNPROTECTED PRIVATE KEY FILE!
# Permissions 0644 for 'keys/archive_ed25519' are too open.
# This private key will be ignored.
ls -l keys/
chmod 600 keys/archive_ed25519
ssh -i keys/archive_ed25519 scp-archive@localhost hostname     # answer "yes" once: a new host key
```

ssh ignores a private key that anybody but its owner can read. The server never saw the key, so
nothing on the server side has to change. With `-v` ssh shows every step of a login when the message
is less clear than this one.

**2. Push, keeping mode and times.** The side written as `account@host:path` is the remote one; a
relative remote path starts in the home directory of that account.

```bash
scp -p -i keys/archive_ed25519 outbox/release-2.4.1.tar.gz outbox/deploy.sh scp-archive@localhost:incoming/
```

`-p` preserves modification time and mode. Without it the copy gets the current time. With
rsync: `rsync -a -e 'ssh -i keys/archive_ed25519' outbox/ scp-archive@localhost:incoming/`
(the trailing slash of `outbox/` means "the contents of").

**3. Pull.** The same command with source and target swapped:

```bash
scp -p -i keys/archive_ed25519 scp-archive@localhost:receipts/release-2.4.0.receipt inbox/
```

**4. Verify on the other side.** A command after the host runs there; the redirection is outside
the quotes, so the output is written here:

```bash
ssh -i keys/archive_ed25519 scp-archive@localhost 'cd incoming && sha256sum deploy.sh release-2.4.1.tar.gz' > remote.sha256
cat remote.sha256
(cd outbox && sha256sum -c ../remote.sha256)      # deploy.sh: OK, release-2.4.1.tar.gz: OK
ssh -i keys/archive_ed25519 scp-archive@localhost 'ls -l --time-style=long-iso incoming'
```

`sha256sum -c` reads such a list and checks every file named in it, which is why the names must be
without a directory here: they are looked up relative to where you run the check.

Typing `-i ...` every time gets old. A `Host` block in `~/.ssh/config` with `HostName`, `User` and
`IdentityFile` gives the connection a short name that `ssh`, `scp` and `rsync` all understand.

**Takeaway:** read the first lines of an ssh error, keep private keys at mode 600, copy with
`scp -p` or `rsync -a`, and trust a transfer only after comparing checksums from both ends.

#!/usr/bin/env bash
# An unfamiliar tool, relcheck, with --help and a manual page, and five release
# directories to run it on. --help is enough for the first report; the list
# separator of RELCHECK_IGNORE and the exit statuses are only in the manual.
set -euo pipefail

base=/opt/course/read-tool-manual
rm -rf "$base"
mkdir -p "$base/bin" "$base/man/man1" "$base/releases"

cat > "$base/bin/relcheck" <<'TOOL'
#!/usr/bin/env python3
"""relcheck - compare release directories with their MANIFEST."""
import fnmatch
import json
import os
import sys

VERSION = "relcheck 0.9.2"
MANUAL = "/opt/course/read-tool-manual/man/man1/relcheck.1"
HELP = """Usage: relcheck [OPTION]... DIR
Compare every release directory below DIR with its MANIFEST file and print one
line per release: name, number of files, bytes and status.

  -a, --all          also check releases whose name starts with a dot
  -f, --format FMT   output format: text (default), tsv or json
  -s, --sort KEY     order of the lines: name (default), files or size
  -q, --quiet        print nothing; the exit status is the result
  -h, --help         show this help and exit
  -V, --version      show the version and exit

The manual page explains the status words, the exit statuses and the
environment variables:  man -l %s
""" % MANUAL


def usage(msg):
    sys.stderr.write("relcheck: %s\nTry 'relcheck --help'.\n" % msg)
    sys.exit(2)


def main(argv):
    show_all, fmt, key, quiet, dirs = False, "text", "name", False, []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a in ("-h", "--help"):
            sys.stdout.write(HELP)
            return 0
        if a in ("-V", "--version"):
            print(VERSION)
            return 0
        if a in ("-a", "--all"):
            show_all = True
        elif a in ("-q", "--quiet"):
            quiet = True
        elif a in ("-f", "--format", "-s", "--sort"):
            if i + 1 >= len(argv):
                usage("option %s needs a value" % a)
            i += 1
            if a in ("-f", "--format"):
                fmt = argv[i]
            else:
                key = argv[i]
        elif a.startswith("--format="):
            fmt = a.split("=", 1)[1]
        elif a.startswith("--sort="):
            key = a.split("=", 1)[1]
        elif a.startswith("-") and a != "-":
            usage("unknown option %s" % a)
        else:
            dirs.append(a)
        i += 1
    if fmt not in ("text", "tsv", "json"):
        usage("unknown format %s" % fmt)
    if key not in ("name", "files", "size"):
        usage("unknown sort key %s" % key)
    if len(dirs) != 1:
        usage("exactly one DIR is required")
    top = dirs[0]
    try:
        names = sorted(os.listdir(top))
    except OSError as e:
        sys.stderr.write("relcheck: %s: %s\n" % (top, e.strerror))
        return 3

    ignore = [p for p in os.environ.get("RELCHECK_IGNORE", "").split(":") if p]
    rows = []
    for name in names:
        path = os.path.join(top, name)
        if not os.path.isdir(path) or (name.startswith(".") and not show_all):
            continue
        found = {}
        for f in sorted(os.listdir(path)):
            fp = os.path.join(path, f)
            if f == "MANIFEST" or not os.path.isfile(fp):
                continue
            if any(fnmatch.fnmatchcase(f, p) for p in ignore):
                continue
            found[f] = os.path.getsize(fp)
        status = "OK"
        try:
            listed = {}
            with open(os.path.join(path, "MANIFEST")) as m:
                for line in m:
                    parts = line.split()
                    if len(parts) == 2:
                        listed[parts[1]] = int(parts[0])
            if any(found.get(f) != size for f, size in listed.items()):
                status = "DIFFERS"
            elif any(f not in listed for f in found):
                status = "UNLISTED"
        except OSError:
            status = "NOMANIFEST"
        rows.append({"name": name, "files": len(found), "bytes": sum(found.values()), "status": status})

    if key == "files":
        rows.sort(key=lambda r: (-r["files"], r["name"]))
    elif key == "size":
        rows.sort(key=lambda r: (-r["bytes"], r["name"]))

    if not quiet:
        if fmt == "json":
            print(json.dumps(rows, indent=2))
        elif fmt == "tsv":
            for r in rows:
                print("%s\t%d\t%d\t%s" % (r["name"], r["files"], r["bytes"], r["status"]))
        else:
            for r in rows:
                print("%-12s %3d files %8d bytes  %s" % (r["name"], r["files"], r["bytes"], r["status"]))

    statuses = {r["status"] for r in rows}
    if "NOMANIFEST" in statuses:
        return 4
    if statuses - {"OK"}:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
TOOL
chmod 755 "$base/bin/relcheck"

cat > "$base/man/man1/relcheck.1" <<'MAN'
.TH RELCHECK 1 "2024-05-20" "relcheck 0.9.2" "Release Tools"
.SH NAME
relcheck \- compare release directories with their MANIFEST
.SH SYNOPSIS
.B relcheck
[\fIOPTION\fR]... \fIDIR\fR
.SH DESCRIPTION
.B relcheck
looks at every directory directly below \fIDIR\fR as one release. It compares
the regular files of the release with the file \fBMANIFEST\fR inside it and
prints one line per release: its name, the number of files, their size in
bytes, and a status word. The MANIFEST itself is never counted.
.PP
A MANIFEST has one line per file: the size in bytes, a space, the file name.
.SH OPTIONS
.TP
.BR \-a ", " \-\-all
Also check releases whose name starts with a dot. They are skipped otherwise.
.TP
.BR \-f ", " \-\-format " " \fIFMT\fR
Output format. \fBtext\fR (the default) is aligned for reading. \fBtsv\fR
prints the four fields separated by tabs, without a header, for scripts.
\fBjson\fR prints one array of objects.
.TP
.BR \-s ", " \-\-sort " " \fIKEY\fR
Order of the lines. \fBname\fR (the default) sorts by release name.
\fBfiles\fR and \fBsize\fR put the release with the most files or the most
bytes first; equal releases are ordered by name.
.TP
.BR \-q ", " \-\-quiet
Print nothing. Use it in scripts that only need the exit status.
.TP
.BR \-h ", " \-\-help
Show a summary of the options and exit.
.TP
.BR \-V ", " \-\-version
Show the version and exit.
.SH STATUS WORDS
.TP
.B OK
Every listed file exists with the listed size, and there is no other file.
.TP
.B DIFFERS
A listed file is missing or has another size.
.TP
.B UNLISTED
All listed files are fine, but the release contains a file that the MANIFEST
does not list.
.TP
.B NOMANIFEST
The release has no MANIFEST that can be read.
.SH EXIT STATUS
.TP
.B 0
Every release is OK.
.TP
.B 1
At least one release is DIFFERS or UNLISTED.
.TP
.B 2
Usage error: unknown option, unknown format or sort key, or no DIR.
.TP
.B 3
\fIDIR\fR cannot be read.
.TP
.B 4
At least one release is NOMANIFEST. This status wins over 1.
.SH ENVIRONMENT
.TP
.B RELCHECK_IGNORE
A list of shell patterns, separated by colons as in PATH. A file whose name
matches one of the patterns is left out completely: it is neither counted nor
compared, and it does not make a release UNLISTED. Quote the value, or the
shell expands the patterns before relcheck sees them.
.SH EXAMPLES
Check the releases of an application and read the result:
.PP
.RS
relcheck /srv/app/releases
.RE
.PP
Leave editor backups and core dumps out of the comparison:
.PP
.RS
RELCHECK_IGNORE='*~:core.*' relcheck /srv/app/releases
.RE
.PP
In a script, act on the result only:
.PP
.RS
relcheck \-\-quiet /srv/app/releases || echo "releases need attention"
.RE
MAN

# rel <name> <manifest size of app.bin> <file>=<size>...
rel() {
  local name=$1 listed=$2 f
  shift 2
  mkdir -p "$base/releases/$name"
  for f in "$@"; do
    head -c "${f#*=}" /dev/zero > "$base/releases/$name/${f%%=*}"
  done
  printf '%s app.bin\n%s config.yaml\n' "$listed" "$(stat -c %s "$base/releases/$name/config.yaml")" \
    > "$base/releases/$name/MANIFEST"
}
rel v1.0.0   4000 app.bin=4000 config.yaml=300
rel v1.1.0   5000 app.bin=5000 config.yaml=320 config.yaml.bak=310   # a leftover backup
rel v1.2.0   5250 app.bin=5200 config.yaml=320                       # really broken
rel v1.3.0   6000 app.bin=6000 config.yaml=350
rel .staging 6100 app.bin=6100 config.yaml=350 upload.tmp=9000       # hidden, with a leftover

(cd "$base" && { sha256sum bin/relcheck man/man1/relcheck.1; find releases -printf '%y %s %p\n' | LC_ALL=C sort; } \
  | sha256sum | cut -d' ' -f1) > "$base/.manifest"

chown candidate:candidate "$base"

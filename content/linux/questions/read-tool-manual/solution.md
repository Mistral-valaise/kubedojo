Three places answer almost every question about a command: `--help` (the short list of options), the
manual page (everything, in fixed sections), and for shell builtins `help <name>`.

```bash
ssh {{host}}
cd /opt/course/read-tool-manual
bin/relcheck --help                         # options, and where the manual page is
bin/relcheck releases                       # a first look with the defaults
bin/relcheck --all --format tsv --sort size releases > report.tsv
```

`--help` lists `--all` (releases whose name starts with a dot), `--format tsv` and `--sort size`.
Nothing in it says how to leave files out or what the exit statuses mean; it points to the manual:

```bash
man -l man/man1/relcheck.1                  # -l: open this file instead of searching the system
```

Inside `man` (it is `less`): `/ENVIRONMENT` jumps to the section, `n` to the next hit, `q` quits.
The section says that `RELCHECK_IGNORE` is a list of shell patterns **separated by colons**, and that
the value must be quoted:

```bash
RELCHECK_IGNORE='*.bak:*.tmp' bin/relcheck --all --format tsv --sort size releases > report-clean.tsv
echo 4 > no-manifest-status.txt             # EXIT STATUS: 4 = at least one release is NOMANIFEST
```

- `VAR=value command` sets the variable for this one command only. Nothing has to be exported or
  cleaned up afterwards.
- Without the quotes the shell would try to expand `*.bak:*.tmp` as a file name pattern first.
- `v1.2.0` stays `DIFFERS` in both reports: its `app.bin` really has another size than the MANIFEST
  says. The two `UNLISTED` releases were only noise.
- `echo $?` after a run shows the exit status of that run (1 here). The meaning of the numbers you
  cannot provoke is in the manual.

`man -M /opt/course/read-tool-manual/man relcheck` works too: `-M` names a manual directory to
search. For installed tools it is simply `man <name>`, and `man -k <word>` searches all page titles.

**Takeaway:** `--help` for the options, `man` and `/SECTION` for environment, files and exit status:
read first, then run.

### Security

- The server answers only requests whose `Host` header is a name it is known by: `localhost`,
  `127.0.0.1`, `[::1]`, names below `.localhost`, the address it listens on and the host of
  `--dev-origin`. Other names get `403`. A web page could reach a local kubedojo through a domain of
  its own that resolves to `127.0.0.1` (DNS rebinding), and with it the terminal of a sandbox and the
  tutor. More names are added with `--allowed-hosts` / `KUBEDOJO_ALLOWED_HOSTS` (comma-separated,
  `*.example.com` for subdomains, `*` to turn the check off); the chart has `allowedHosts`.
  `/api/health` and `/api/ready` are answered under any name. A server that listens on every interface
  without `--allowed-hosts` accepts every name and warns at start.
- Requests that change something are rejected with `403` when they come from another origin
  (`Sec-Fetch-Site`, or `Origin` against `Host` for browsers that do not send it).
- A request body is read only when it is declared as JSON. A form on any web page can post
  `text/plain` to a local server; such a body is now answered with `415`.
- Every response carries a Content-Security-Policy (scripts, styles, images and fonts from the server
  only, no framing) and `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options`,
  `Cross-Origin-Opener-Policy` and `Cross-Origin-Resource-Policy`.
- Session IDs come from the system's random source and have 12 characters instead of 8. Sessions that
  exist keep their ID.
- The data directory is created with mode `0700`, and the database, its backups, the database a
  restore keeps and the settings with `0600`. What an older release created is changed when the
  server starts. `<data>/observability` is left alone: Grafana reads it from its container.
- The ssh key of a session and the kubeconfigs of its clusters reach the terminal container on the
  standard input of `docker exec`, no longer as environment arguments that `ps` shows.
- A client has 15 seconds to send a request body, and an unused connection is closed after two
  minutes. Requests that take long (grading, the terminal) are not limited.
- A panic in a request is answered with `500` and logged with its stack; before, the connection was
  closed without an answer.

### Changed

- **Breaking, hosted mode:** the chart tells the server which names it is reached under: the host of
  the Ingress and the names of the Service. If people reach kubedojo under another name or an IP
  address (a second DNS name, a NodePort or LoadBalancer address, a cluster domain other than
  `cluster.local`), list it in `allowedHosts`, or they get `403`. A port-forward to `localhost` works
  as before.
- **Breaking, API:** a request with a body needs `Content-Type: application/json`, or it is answered
  with `415`. The UI always sent it. Requests without a body (start, finish, prepare, grade) need no
  header.
- A response of the API to a request that is rejected for its `Host`, its origin or its
  `Content-Type` is JSON of the form `{"error": "..."}`, like every other error.

### Fixed

- `kubedojo serve --data <directory>` failed when the directory did not exist yet: the lock was taken
  before the directory was created.

# Reproduce the SQLite workload

This synthetic workload is excluded from ordinary builds and tests. Each package
creates its own temporary SQLite database with the same 50,000 messages, 200
64-KiB attachments, and 20,000 Inbox links owned by the measured account. Every
100th subject contains `needle`. Setup and the initial IMAP SELECT are outside the
measured loops; POP3 opens a fresh transaction-state snapshot each iteration.

From `server/`, at the baseline benchmark commit or at the optimized commit:

```sh
GOTOOLCHAIN=go1.27.4 go test -tags perfseed -run '^$' -bench '^BenchmarkPMail' \
  -benchmem -benchtime=3x -p 1 ./services/list ./listen/imap_server ./listen/pop3_server
```

On the optimized commit, inspect the actual list query plans with:

```sh
GOTOOLCHAIN=go1.27.4 go test -tags perfseed -run '^TestPMailSQLiteQueryPlans$' \
  -v ./services/list
```

The Web measurements exercise the database service rather than HTTP transport or
JSON encoding. The offset list keeps exact counts on both revisions so the
comparison does not attribute skipped `COUNT` work to projections. The seed writes
projections before timing on the optimized revision to model a completed upgrade.

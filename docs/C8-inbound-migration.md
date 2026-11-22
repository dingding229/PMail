# Planned C8 inbound analysis persistence

This design and the planned OpenAPI operations do not activate scoring, training,
scanning, quarantine, or new routes. Use the OpenAPI request/response `examples`
for MSW. Error responses from the new endpoints localize `errorMsg` and expose
`data.code`; clients must not parse the localized message. Existing attachment
routes retain their legacy errors and add only the planned risk errors.

- Add user settings keyed by `user_id`, a singleton inbound settings row, and
  owner-scoped rules with a unique index on `(user_id, action, kind, value_hash)`.
  Normalize addresses/domains and keyword whitespace before hashing; serialize
  create per user to enforce the 500-row cap under concurrent requests. Scanner
  endpoints and DNSBL zones are administrator configuration only.
- Put content risk and extracted codes on immutable email/content records (with
  a scan version and stable attachment indices); keep verdict, scores, reasons,
  and training state per owner and email. Never expose another owner's verdict
  through a shared message or apply its labels to that owner's inbox.
- Keep Bayesian counters per user. A spam toggle updates the classification,
  counters and the caller's mailbox links atomically. Use the previous label as
  the compare/update guard; identical requests must not retrain. A bulk request
  validates every owner link before making changes.
- Add portable tables/nullable columns first via the existing schema sync for
  SQLite, MySQL and PostgreSQL. Legacy rows start with `spam: null`, `risk:
  {level: none, findings: []}` and `verification_code: null`; absence of a scan
  is not a clean scan verdict internally. Do not train on legacy mail. If enabled,
  analyze old mail in resumable PK batches off the request path, with bounded
  scan size and concurrency. Do not persist verification codes to logs. Excerpts
  are truncated after escaping; download acknowledgements are not authorization.
- Enforce high-risk download/preview consistently on new and legacy attachment
  routes, including HEAD and inline aliases; after explicit acknowledgement serve
  high-risk bytes as a download, not inline. Rollback disables analysis reads and
  writes without dropping tables or changing existing mailbox data. Deploy with
  behavior disabled until migrations finish, then enable by policy.

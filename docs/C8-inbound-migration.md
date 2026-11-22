# C8 inbound analysis persistence

The runtime OpenAPI operations use localized `errorMsg` and stable `data.code`
for errors. The models are additive; the migration never scans existing mail.

- `email.inbound_analysis` stores versioned shared evidence (base reasons, code,
  risk findings, and the authoritative high-risk part indices). Only the
  bounded list/detail projections reach the API. `user_email.inbound_spam`
  stores the link owner's verdict, score and reasons. IMAP copies inherit the
  same owner's classification; manually relabeling applies to that owner's
  copies only. An administrator's optional delegated mail read does not acquire
  another owner's personal verdict.
- `inbound_settings` holds global settings (singleton ID 1); every subsequent
  analysis reads committed settings. `spam_preference` holds user overrides.
  `spam_rule` stores normalized owner rules with a unique key and a transactional
  500-rule cap. No configuration or mail bytes are written to scanner warnings.
- `spam_training`, `spam_totals` and `spam_token` retain per-owner training labels
  and hashed token-presence counts. Label reversal undoes the previous sample;
  re-submitting the same label does not re-train. Bulk actions validate and
  mutate in one transaction. Learning retains counts even if the sampled mail
  is later deleted, and does not store whole original bodies in training tables.
- `db.SyncSchema` adds nullable evidence columns and these tables on SQLite,
  MySQL and PostgreSQL. Legacy rows have null `spam`, `risk`, and
  `verification_code` until explicitly reclassified (a manual move adds only
  the owner's spam label). An inspected clean message, unlike one not scanned,
  has risk `{level: none, findings: []}`. The offline SQLite/MySQL-to-PG importer
  preserves these tables if they exist and skips them when importing old schemas.
- High-risk confirmations never authorize access, including on legacy aliases;
  confirmed bytes are served as a forced download. No scan or risk data is
  written back into original MIME. A rollback can restore the prior snapshot;
  do not assume an old binary enforces C8's attachment protection even if the
  additive columns remain present.

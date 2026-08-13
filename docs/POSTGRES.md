# PostgreSQL (Enterprise tier)

AlertHub runs on two storage backends from one code path:

| Tier | Backend | Use |
|------|---------|-----|
| **Starter** (default) | SQLite (`modernc.org/sqlite`, pure Go) | single binary, self-host, family/small-team |
| **Enterprise** | PostgreSQL (`jackc/pgx`) | multi-tenant at scale, Row-Level Security, HA |

The schema and all queries are identical across both; only the DDL and the
placeholder style (`?` vs `$1`) differ, handled inside `internal/store`.

## Switching to Postgres

```sh
export ALERTHUB_DB_DRIVER=postgres
export ALERTHUB_DB_DSN="postgres://USER:PASS@HOST:5432/alerthub?sslmode=require"
./alerthub
```

On boot the server logs `store: postgres (enterprise tier)`, creates the schema
(idempotent), seeds the default org + admin, and installs the RLS policies. With
`ALERTHUB_DB_DRIVER` unset (or `sqlite`) it uses `ALERTHUB_DB_PATH` as before.

## Tenant isolation — two layers

1. **Application layer (always on, both backends).** Every tenant query carries an
   explicit `org_id`, resolved per request from the `X-Org-Id` header / API-key
   org / default org. This is the primary wall and is verified on both SQLite and
   Postgres.
2. **Row-Level Security (Postgres, defense-in-depth).** `alerts` has `ENABLE` +
   `FORCE ROW LEVEL SECURITY` with a policy that filters on the transaction GUC
   `app.current_org`:

   ```sql
   USING      (org_id = NULLIF(current_setting('app.current_org', true), '')::bigint)
   WITH CHECK (org_id = NULLIF(current_setting('app.current_org', true), '')::bigint)
   ```

   An unset/empty GUC collapses to `NULL` → **zero rows (fail-closed)**. The `USING`
   clause guards reads, `WITH CHECK` guards writes (an INSERT/UPDATE whose `org_id`
   differs from the GUC is rejected). This wall holds even if a query ever forgets
   its `WHERE org_id`.

   `service_accounts` is **deliberately not** under RLS: API-key auth
   (`requireScope`) must look a key up by `token_hash` *before* any org is known, so
   an org-scoped policy there would break authentication. Its tenant isolation is
   the app-level `org_id` filter on create/list/delete (verified on both backends).

### Activating the RLS wall

RLS is **installed** automatically but only **enforced** for non-superuser roles —
PostgreSQL superusers (and `BYPASSRLS` roles) bypass RLS even with `FORCE`. To make
it active:

1. Connect AlertHub with a **least-privilege role** (not the DB owner, not a
   superuser, no `BYPASSRLS`):

   ```sql
   CREATE ROLE alerthub_app LOGIN PASSWORD '…' NOSUPERUSER NOBYPASSRLS;
   GRANT CONNECT ON DATABASE alerthub TO alerthub_app;
   GRANT USAGE ON SCHEMA public TO alerthub_app;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO alerthub_app;
   GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO alerthub_app;
   ```

   Run the **migration/bootstrap** as the owner (it `TRUNCATE`s/`ALTER`s), then run
   the **app** as `alerthub_app`.

2. The GUC is set per request, automatically. `Store.BeginOrg(ctx, orgID)` opens a
   transaction and runs `set_config('app.current_org', orgID, true)` (transaction-
   local, i.e. `SET LOCAL`); `api.Server.inOrg` wraps every `alerts` read/write —
   from HTTP handlers and background producers (EEW/CAP) alike — in that
   transaction and commits/rolls back around it. Because it is transaction-local, a
   pooled connection never leaks one request's org to the next.

   > **RLS is now the live enforcement layer**, not just a latent wall: the
   > per-request `SET LOCAL` is wired via `Store.BeginOrg` +
   > `api.Server.inOrg`. App-level `org_id` filters remain the primary isolation;
   > RLS is the defense-in-depth second wall (both directions — see `WITH CHECK`).
   > Proven by `store/pg_test.go` (`TestPostgres_RLS_LiveViaBeginOrg`,
   > `TestPostgres_RLS_WriteCheck`) and `api/api_test.go`
   > (`TestPostgres_PublishHistoryE2E`). On SQLite, `BeginOrg` is a no-op and the
   > app-level filter is the isolation.

## Tests

Postgres + RLS integration tests live in `internal/store/pg_test.go` and **skip**
unless a disposable database is provided:

```sh
createdb alerthub_test
ALERTHUB_TEST_PG_DSN="postgres://$USER@localhost:5432/alerthub_test?sslmode=disable" \
  go test ./server/internal/store/... -run TestPostgres -v
```

- `TestPostgres_TenantIsolation` — org-scoped Save/History on real Postgres.
- `TestPostgres_RLS` — a non-superuser role sees only its org's rows on a bare
  `SELECT * FROM alerts`, and zero rows when the GUC is unset/empty/bogus.
- `TestPostgres_BeginOrg_SetsGUC` — `BeginOrg` sets `app.current_org` and its
  tx-bound Store reads inside the transaction.
- `TestPostgres_RLS_LiveViaBeginOrg` — end-to-end: under a least-priv role, a bare
  `SELECT` inside `BeginOrg(org)` returns only that org's rows (read wall is live).
- `TestPostgres_RLS_WriteCheck` — a write whose `org_id` ≠ the GUC is rejected by
  `WITH CHECK` (write wall is live).
- `TestPostgres_ServiceAccountsNotRLS` — asserts `service_accounts` is not RLS-gated
  while `alerts` is.

The HTTP-level E2E `api/api_test.go:TestPostgres_PublishHistoryE2E` (also gated on
`ALERTHUB_TEST_PG_DSN`) drives the real handlers so the publish/history paths run as
actual `inOrg` transactions on Postgres.

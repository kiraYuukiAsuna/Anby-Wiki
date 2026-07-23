# M7-T08 Deployment and Migration Runbook

## Scope

This runbook deploys the platform-neutral OCI targets `api`, `worker`, `web`,
and `migrate` through `infra/deploy/compose.production.yml`. Compose is the
reference manifest; another scheduler may consume the same images, environment
contract, secret files, probes, and rollout order.

The reference Compose deployment provides ordered component replacement, not
replica-by-replica zero-downtime orchestration. A Kubernetes/Nomad adapter must
preserve the same migration gate and use its native rolling-update primitive.

## Build and Publish

Build each target with one immutable release identifier and publish by digest:

```sh
VERSION=2026.07.23.1
VCS_REF="$(git rev-parse HEAD)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

for target in api worker web migrate; do
  docker build \
    --target "$target" \
    --build-arg VERSION="$VERSION" \
    --build-arg VCS_REF="$VCS_REF" \
    --build-arg BUILD_DATE="$BUILD_DATE" \
    -t "registry.example.com/anby/anby-wiki-$target:$VERSION" .
  docker push "registry.example.com/anby/anby-wiki-$target:$VERSION"
done
```

Do not deploy mutable `latest` tags. Resolve each pushed image and the pinned
unprivileged Nginx image to its registry digest, then set every `*_IMAGE` value
to a full `name@sha256:...` reference. Production promotion records those
digests, Git revision, migration target, compatibility window, and CI result.

## Runtime Security

All application containers run as non-root with a read-only root filesystem,
all Linux capabilities dropped, `no-new-privileges`, bounded `/tmp` tmpfs, and
healthchecks. Nginx uses its unprivileged image and listens on container port
8080. The host bind defaults to `127.0.0.1`; terminate TLS at the platform load
balancer and do not expose the internal application network.

The M7-T03 gateway controls remain active: security headers, upload/body limits,
rate limits, exact trusted origins, secure cookies, and removal of spoofable
identity headers. The API `/readyz`, Worker `/metrics`, Web TCP listener, and
Nginx `/gateway-healthz` are rollout gates. M7-T04 metrics and OTLP variables
remain part of the deployment contract.

## Environment Contract

Copy `infra/deploy/.env.example` to a protected location outside the repository.
The file contains non-secret routing and release metadata only.

| Variable | Required | Purpose |
| --- | --- | --- |
| `RELEASE_ID` | yes | Human-readable release confirmation identifier |
| `API_IMAGE`, `WORKER_IMAGE`, `WEB_IMAGE`, `MIGRATE_IMAGE`, `NGINX_IMAGE` | yes | Full immutable `name@sha256:...` image references |
| `APP_NETWORK` | yes | External network connected to managed dependencies |
| `MIGRATION_EXPECTED_VERSION` | yes | Exact database version after migration |
| `SCHEMA_MIN_COMPATIBLE_VERSION`, `SCHEMA_MAX_COMPATIBLE_VERSION` | yes | Schema range supported by this release |
| `S3_ENDPOINT`, `S3_BUCKET`, `MEILI_URL`, `MEILI_INDEX` | yes | Object/search service endpoints |
| `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_REDIRECT_URL` | yes | Production OIDC metadata |
| `TRUSTED_ORIGINS` | yes | Comma-separated exact HTTPS browser origins |
| `OTEL_*`, `OBSERVABILITY_DB_INTERVAL` | no | M7-T04 tracing/metrics settings |
| `AI_IMPORT_ENABLED` | no | Worker AI import switch, default false |

Secrets are external platform objects mounted as files. Their names are supplied
through `DATABASE_URL_SECRET`, `REDIS_URL_SECRET`, `S3_ACCESS_KEY_SECRET`,
`S3_SECRET_KEY_SECRET`, `MEILI_API_KEY_SECRET`, and
`OIDC_CLIENT_SECRET_SECRET`. The container entrypoint maps only the supported
`*_FILE` paths into process environment and never prints values. Secret files
must contain one raw value without shell quoting. Do not place credentials in
the env file, image layers, Compose command line, CI logs, or metrics.

When enabling AI import, provide an orchestrator override that mounts an
`AI_API_KEY_FILE` secret and supplies `AI_BASE_URL`/`AI_MODEL`; do not add the
key to the checked-in reference manifest.

## Preflight

1. Confirm M7-T03 security and dependency scans passed.
2. Confirm M7-T04 alerts are loaded and API/Worker metrics are being scraped.
3. Take an M7-T06 PostgreSQL and object-storage backup before a risky migration.
4. Confirm the M7-T07 doctor is healthy on the current release.
5. Verify the release migration target equals the highest checked-in migration.
6. Verify both old and new releases support the migration transition. Use
   expand/contract migrations; never remove a column or behavior still used by
   the old release during a rolling window.
7. Set `DEPLOY_CONFIRM=DEPLOY:<RELEASE_ID>` only for the intended release.

Run static/config validation:

```sh
make deploy-config-check
DEPLOY_ENV_FILE=/secure/anby-wiki.env sh scripts/deploy.sh config
```

## Migration Gate

Application startup never runs migrations. The only production path is an
explicit one-shot command:

```sh
DEPLOY_ENV_FILE=/secure/anby-wiki.env \
DEPLOY_CONFIRM=DEPLOY:2026.07.23.1 \
sh scripts/deploy.sh migrate
```

The gate performs, in order:

1. Validate positive version numbers and `min <= expected <= max`.
2. Run `wiki-migrate up`; golang-migrate rejects an existing dirty state.
3. Run `wiki-migrate check EXPECTED MIN MAX`.
4. Require exact target version, `dirty=false`, and target inside the image
   compatibility window.
5. Run M7-T07 `wiki-doctor -format json`; any error/critical issue stops release.

Never use `migrate down` in production. A dirty migration is a hard stop: retain
the database and logs, prepare an idempotent forward-fix migration under the
single migration-sequence owner, test it on an M7-T06 restored copy, and rerun
the explicit gate.

## Ordered Rollout

Execute:

```sh
DEPLOY_ENV_FILE=/secure/anby-wiki.env \
DEPLOY_CONFIRM=DEPLOY:2026.07.23.1 \
sh scripts/deploy.sh deploy
```

The fixed order is:

1. Pull images and run migration gate plus doctor.
2. Replace API and wait for `/readyz`.
3. Replace Worker and wait for `/metrics`.
4. Replace Web and wait for its listener.
5. Replace Nginx and wait for `/gateway-healthz`.

The script uses `set -e`; a pull, migration, doctor, start, or health failure
prevents every later step. Do not manually skip ahead. After success, inspect
API error/latency, queue depth, projection lag, import failures, and trace export
from M7-T04 before closing the release.

## Failure and Rollback

Before migration completion, fix configuration/image/secret access and rerun.
After migration but before all services are healthy, leave healthy new
components in place, diagnose the failed component, and prefer a forward-fixed
image compatible with the migrated schema.

Application rollback is allowed only when the old image explicitly supports the
current database version:

```sh
# Set the application image references and RELEASE_ID to the previous release, but keep
# MIGRATION_EXPECTED_VERSION equal to the current live database version.
DEPLOY_ENV_FILE=/secure/anby-wiki-rollback.env \
DEPLOY_CONFIRM=DEPLOY:previous-release \
sh scripts/deploy.sh rollback
```

Rollback runs `check` and doctor but never `up` or `down`, then uses the same
API -> Worker -> Web -> Nginx order. If the old compatibility window excludes
the live schema, rollback is forbidden; ship a forward fix. For unrecoverable
data damage, follow `Docs/runbooks/backup-restore.md`: restore into new named
resources, verify hashes, run doctor, rebuild Projection/Search, switch traffic,
and preserve the damaged resources for investigation.

## Seed

The single `000001_initial_schema` migration contains required deterministic
bootstrap reference rows and is part of every environment. Arbitrary
fixture/performance seed is never automatic and is forbidden in production.

The optional `seed` profile invokes the existing M7-T05 performance tool, which
requires a separately named performance database and its own confirmation:

```sh
DEPLOY_ENV_FILE=/secure/anby-wiki-staging.env \
SEED_CONFIRM=SEED_NON_PRODUCTION_PERF_DATABASE \
sh scripts/deploy.sh seed
```

The command rejects `DEPLOY_ENV=production`; the tool also ignores
`DATABASE_URL`, requires `PERF_DATABASE_URL`, and verifies the isolated database
name before writing.

## Known Blockers

- Local real OCI builds and `docker compose config` are blocked when Docker is
  unavailable. Static Dockerfile/manifest/shell/gate/order checks still run;
  the CI `deploy` job is the authoritative real-build gate.
- M7-T03's `sharp 0.34.5` production high vulnerability remains a release
  blocker until a compatible fixed dependency is available.
- M7-T07 still does not stream and verify object-storage bytes; use the M7-T06
  object manifest verification for release/restore evidence.

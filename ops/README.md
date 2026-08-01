# Production deployment

Production has one supported deployment source: the GitHub-connected `main`
branch of `lingozhi/new-api`.

Do not run `railway up` against the production service. Railway accepts a local
directory upload without checking whether its contents are older than the
currently deployed Git revision.

The Docker build executes `verify-production-build-source.sh` before the
application image is completed. It identifies the production target by its
immutable Railway project, environment, and service IDs, so renaming an
environment cannot bypass the check. On that target it requires the GitHub
deployment metadata to identify:

- repository owner `lingozhi`;
- repository name `new-api`;
- branch `main`;
- a valid 40-character Git commit SHA.

Railway only provides these Git variables when a deployment originates from its
connected GitHub source. A CLI directory upload made from a checkout containing
this guard therefore fails before the application image can be built or
activated. Local, development, and staging builds are unaffected.

This repository check is defense in depth, not the production trust boundary. A
directory upload controls its own Dockerfile, so an old or modified checkout can
omit the guard entirely. Enforce Git-only production access in Railway with a
restricted production environment and non-admin operator roles. If the Railway
workspace plan does not provide Environment RBAC, repository code cannot fully
disable owner/editor CLI uploads; keep production operators to the minimum and
do not expose project or account deployment tokens to routine automation.

Run the guard regression checks with:

```bash
./ops/verify-production-build-source_test.sh
```

Intentional production rollbacks must deploy a reviewed commit from the
connected GitHub repository. Do not use a local directory upload as a rollback
mechanism.

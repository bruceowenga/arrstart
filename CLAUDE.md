# Project rules

- Read docs/PRD.md and docs/BUILD-PLAN.md before starting. Work on one WP at a time.
- Commits: Conventional Commits with a `Refs:` footer listing PRD IDs.
- Run `make check` before declaring work done. Run `make e2e` for wiring/doctor changes.
- Verify app API behavior against the pinned images in real containers. Do not rely on memory.
- Never record "X is impossible" without a spec-following attempt and a check of the OpenAPI spec. Write "not found at probed paths" until a second attempt confirms it.
- Never add, name, or preconfigure real indexers or content sources. Use the mock indexer.
- Never commit secrets, real `.env` files, or generated compose files from real runs.
- Pin image tags. Do not use `latest` in generated output.
- Generated compose and `.env` must stay plain and hand-editable (no lock-in).
- Finish every task with: changed, verified, NOT verified, next step.
- Update PRD/BUILD-PLAN checkboxes and docs/build-log.md in the same PR.
- `Generate` returns `[]byte` for compose and `.env`; convert with `string()` before `strings.Contains` in tests. Comparing whole outputs with `bytes.Equal` (golden tests) is fine.
- Golden fixtures are produced only with `-update` after the generator compiles; never hand-write them. Golden `.env` files are tracked through the `.gitignore` exception under `internal/generator/testdata`.

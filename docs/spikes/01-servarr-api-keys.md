# Spike S1: Servarr API key injection via environment variables

## Goal

Confirm that env-var API key injection (`APP__SECTION__KEY` style) works for Sonarr, Radarr, Prowlarr, and identify the config.xml fallback path — per PRD requirement **A1**:

> Pre-generate API keys and inject via environment variables where the app supports it (Servarr apps support `APP__SECTION__KEY` style overrides; verify per version). Fall back to reading `config.xml` from the config volume.

## Method

Searched the Servarr Wiki (mirrored on GitHub) for Sonarr, Radarr, and Prowlarr environment-variable documentation. Did **not** run against real pinned containers yet — that verification remains open.

## Confirmed findings

### Pattern

All three apps follow the pattern:

```
<APPNAME>__<NAMESPACE>__<CONFIGITEM>
```

where `__` (double underscore) is the .NET configuration key separator. Variables override entries in `config.xml`. Names are case-sensitive. A restart is required after changing them.

Source: Servarr Wiki mirrors — `lazyassadmin/Servarr_Wiki` (sonarr/radarr) and `bakerboy448/ServarrWiki` (prowlarr).

### API key / authentication env vars

| App | Pattern | Auth vars |
|-----|---------|-----------|
| Sonarr | `SONARR__NS__ITEM` | `SONARR__AUTH__APIKEY`, `SONARR__AUTH__ENABLED`, `SONARR__AUTH__METHOD`, `SONARR__AUTH__REQUIRED`, `SONARR__AUTH__TRUSTCGNATIPADDRESSES` |
| Radarr | `RADARR__NS__ITEM` | `RADARR__AUTH__APIKEY`, `RADARR__AUTH__ENABLED`, `RADARR__AUTH__METHOD`, `RADARR__AUTH__REQUIRED`, `RADARR__AUTH__TRUSTCGNATIPADDRESSES` |
| Prowlarr | `PROWLARR__NS__ITEM` | `PROWLARR__AUTH__APIKEY`, `PROWLARR__AUTH__ENABLED`, `PROWLARR__AUTH__METHOD`, `PROWLARR__AUTH__REQUIRED`, `PROWLARR__AUTH__TRUSTCGNATIPADDRESSES` |

Relevant namespaces: `APP__`, `AUTH__`, `LOG__`, `POSTGRES__`, `SERVER__`, `UPDATE__`.

### PRD `APP__SECTION__KEY` notation

The PRD writes `APP__SECTION__KEY` as a placeholder template where `APP` is the app-name prefix (`SONARR`, `RADARR`, `PROWLARR`). Confirmed consistent — e.g. `SONARR__AUTH__APIKEY` = `APP=SONARR`, `SECTION=AUTH`, `KEY=APIKEY`.

### Config.xml fallback (not yet confirmed)

The wiki pages state that environment variables override `config.xml` entries, but do **not** document:
- The `config.xml` file path per OS / per Docker image
- The XML element name for the API key
- Whether the key is stored in plaintext or obfuscated in `config.xml`

These need verification against real pinned containers (see Open below).

## Open / unverified

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| config.xml path for each app on Linux and in the pinned Docker image | Fallback reads `config.xml` from the config volume | Launch pinned image, inspect `/config/config.xml` |
| XML element name for stored API key | Needed to read/write the key via fallback | Inspect `config.xml` after setting API key via UI |
| Whether `APP__AUTH__APIKEY` is accepted at runtime vs. only at first start | Determines if rewire can change the key without restart | Test against running container |
| Behavior across pinned Sonarr / Radarr / Prowlarr versions | API may change between versions | Test each pinned version |
| Does auth need to be explicitly `ENABLED` for APIKEY to take effect? | A1 says enable authentication on arr apps | Verify `AUTH__ENABLED=true` interaction |

## Recommendation

For the wiring engine (WP3):

1. **Primary:** Inject `SONARR__AUTH__APIKEY` / `RADARR__AUTH__APIKEY` / `PROWLARR__AUTH__APIKEY` via compose `environment:` at container creation, alongside `AUTH__ENABLED=true` and `AUTH__METHOD=external` (or appropriate method).
2. **Fallback:** On `rewire` or if env injection fails, read `config.xml` from the app's `<root>/config/<app>/config.xml` to recover the existing key.
3. **Idempotency:** If the injected key matches what's already in `config.xml`, skip reconfiguration (WP3.9 `rewire` should be a no-op when nothing changed).

## Sources

- Servarr Wiki, Sonarr environment variables: https://wiki.servarr.com/sonarr/environment-variables (mirrored at lazyassadmin/Servarr_Wiki)
- Servarr Wiki, Radarr environment variables: https://wiki.servarr.com/radarr/environment-variables (mirrored at lazyassadmin/Servarr_Wiki)
- Servarr Wiki, Prowlarr environment variables: https://wiki.servarr.com/en/prowlarr/environment-variables (mirrored at bakerboy448/ServarrWiki)
- NixOS Homelab docs, Arr API configuration: https://carpenike.github.io/nix-config/arr-services-api-key-configuration/
- Prowlarr env var config implemented: https://github.com/Prowlarr/Prowlarr/issues/777

## Decision needed

- Confirm pinned image tags for Sonarr, Radarr, Prowlarr (WP0.9) before running verification containers.
- Decide whether `APP__AUTH__APIKEY` env injection alone is sufficient, or whether we also write `config.xml` directly (config.xml read/write is more invasive but works on apps that don't honour env vars).

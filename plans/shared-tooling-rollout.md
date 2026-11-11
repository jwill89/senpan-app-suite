# shared-tooling rollout

The passages of shared-tooling's cross-project plan (`PLAN.md` at v2.6.0) that name this project, moved here word for word on 2026-11-10 when shared-tooling became project-agnostic. The portfolio-wide rest is in `PLAN.md` beside the project folders.

## Decisions so far

The standardization work starts from these, so it doesn't reopen them. All were decided on November 5, 2026.

- huma for OpenAPI everywhere. It generates the spec from code with readable operation and schema names, so Senpan drops its hand-written spec and kin-openapi.

Decided later the same day, while seeding shared-tooling:

- A failed start restores the database snapshot only where a project opts in: Kindred Nights, Home Library and Senpan do; the Gallery doesn't, since uploads made during the deploy would be lost.

Decided 2026-11-06, after the Gallery's pilot:

- After the pilots, the rollout order is Senpan, Marxbot, TarotGen.io, then `standards/` and the rest. Senpan and Marxbot go first because they have no scheduled backup; TarotGen.io no longer waits for a port. The three get their CLAUDE.md pointer to the standards once `standards/` exists.

Decided 2026-11-07, where the projects' own docs disagreed (55 questions, now rules in `standards/`):

- **Accessibility:** the target is WCAG 2.2 AAA, as Kindred Nights, Home Library and Senpan's themes already state it, falling back to AA only where AAA can't be met, with each such place a dated line in the project's decision log. axe in the end-to-end suites enforces the AA floor; AAA contrast is held by each project's own contrast test. (Corrected 2026-11-07: v2.1.0's guide named AA as the target.)

- **Interface:** layout with Tailwind or with a stylesheet of object classes is each project's choice. Tints use `color-mix()`; CSS never names `black` or `white`. Each animation honours reduced motion itself. Touch targets are at least 44 by 44 px. A new record is made in a dialog. A service worker never caches an API answer; it may cache content-hashed files and thumbnails, and only an installable app precaches its shell (revised the same day, for the Gallery's thumbnails and Senpan's installable app). A list's state lives in the URL, with identity in the path. A dropped live feed shows a banner. A wide table may scroll inside its panel.

## Project by project

| Project | Stack now | Verdict |
| --- | --- | --- |
| Kindred Nights, Senpan, Home Library | Go + Vue / Vite / TS + SQLite | Keep |
| Venue Tools, Kindred/Senpan/TarotGen plugins | C# / .NET | Dalamud requires it |

## Where the real gains are

### 1. Stay on Vue; Vapor is a small gain, later

**How much it helps.** Not much that visitors would notice:

- **Where you might feel it:** screens that redraw many items often, such as Home Library's list (thousands of books filtered and sorted in the browser), Senpan's live bingo boards and Kindred's staff consoles, mostly on older phones.

### 5. Small inconsistencies

- **Two SQLite drivers:** modernc in four projects, ncruces in two. Senpan's ncruces choice is correctly documented against `MemoryDenyWriteExecute`, but neither project needs ncruces: both use it only for a per-connection pragma hook and one interrupt check, which modernc also covers. Standardize on modernc, which is equally safe under that setting.
- **Two OpenAPI approaches:** huma (code-first) in Kindred Nights and Home Library, kin-openapi with a hand-written spec in Senpan. Settled: huma, so Senpan moves to it.

## Real-time: WebSockets, streams or REST

| Project | Today | Best fit | Why |
| --- | --- | --- | --- |
| Senpan App Suite | WebSockets | SSE, as Kindred's feed (moving in its release C) | Calls flow one way; claims and every other write go over REST, and the server discards what a socket sends (found 2026-11-09) |

## Shared packages

**C# and PowerShell both earn a folder.** The four plugins use three setups today: Kindred's `Directory.Build.props`, settings inside Senpan's and TarotGen.io's project files, and nothing in Venue Tools, though Senpan's file says it means to match Kindred. For PowerShell, a shared deploy module beats a shared lint file: the 14 deploy scripts total 4,321 lines, and helpers like `Resolve-PuttyTool`, `Write-Step` and `Fail` are copied into 11 of them.

**Write the package choices down, then enforce them.** standards/packages.md holds a Use / Don't use table per stack, and the linter fails the build where it can:

- **Go:** golangci-lint's `depguard` rule rejects a banned import, such as ncruces once Senpan and Modem Monitor move to modernc.

## Testing standards

| Stack | Today | Gap | shared-tooling adds |
| --- | --- | --- | --- |
| Vue / TS | Vitest in 8 of 9 front ends, about 250 test files | A floor only in TarotGen.io; Fingerprint Generator has no tests; two DOM libraries (happy-dom, and jsdom in Senpan and TarotGen.io) | `test/web`: a Vitest preset on jsdom, with v8 coverage and a floor |

No test relies on a happy-dom-only API, so the switch is one config line per project plus a test run. The Gallery, Home Library, TarotGen.io and Kindred's markdown test move over; Senpan already uses jsdom. Tests that touch no DOM, like Fantasy Dossier's, stay on Vitest's faster `node` environment.

## Project gaps

Most gaps are missing CI, Dependabot and post-deploy checks. The riskiest were the three SQLite apps with no scheduled backup, the Gallery, Senpan and Marxbot, until their 2026-11-06 changes: each deploy now installs a daily backup. No secrets are committed: the two tracked `.npmrc` files hold only the registry address, and every deploy config and `.env` file is ignored.

| Project | Missing | Note |
| --- | --- | --- |
| Senpan App Suite | None once its 2026-11-06 changes land | Rolled out first after the pilots |

## Where to start

Do what's urgent first, settle the decisions, build the tooling, and only then migrate. Migrating first would mean writing new code, such as Senpan's huma API, to today's mixed conventions, then reworking it once the standards exist.

<!-- markdownlint-disable MD029 -->

1. **Protect money and data first.**
   - Scheduled database backups for Senpan, the Gallery and Marxbot: they become a shared piece, from Kindred's backup timer, and land with each project's rollout in step 6, the Gallery's first (decided 2026-11-06).

<!-- -->

3. **Gather the standards into `standards/` in this repo** (github.com/jwill89/shared-tooling, public). Claude Code sorts each rule as shared, project-only or conflicting and brings you the conflicts. It comes after the rollouts of Senpan, Marxbot and TarotGen.io (decided 2026-11-06), which take their CLAUDE.md pointer then.
   - Where a decision changes how an existing project works, the change is that project's own follow-up:
     - **Senpan**, with its move to huma: the backend reorganised from layers (`server` handlers, `bingo`, `store`, `model`) into domain packages with thin handlers, `server` alone being 97 files; RFC 9457 errors, 422 for validation, camelCase JSON, openapi-typescript in place of tygo, goose (with `user_version`) in place of its own migration blocks, a write pool and a read pool, the front end embedded in the binary, one app version, Vitest specs in `src/__tests__/` with the config in `vite.config.ts`, and the shared `vue/no-static-inline-styles` rule back on.

<!-- -->

6. **Roll out one repo at a time**, on `main`, with no separate branches. Everything shared-tooling can do lands in one pass: swap in the shared configs, move its deploy script onto the shared module, turn on its scheduled backup and database pull, and add its basic end-to-end suite, together (decided 2026-11-05, after Kindred's pilot did them on two branches; backups added 2026-11-06). Point its CLAUDE.md at the standards, trim what moved there, and close that project's gaps (see Project gaps). Small fixes ride along: the SQLite switch in Senpan and Modem Monitor, jsdom, and Playwright for the résumé site. After the pilots, the order is Senpan, Marxbot, TarotGen.io, then the rest (decided 2026-11-06).
   - **Senpan** (done 2026-11-06, `20605df`, CI green with every shared job and its end-to-end suite; deployed on the module 2026-11-07, on v2.0.0):
     - Everything shared-tooling can do, in one pass: the shared configs and CI, Dependabot, the deploy on SharedDeploy with an HTTP smoke check and rollback, the daily backup, the database pull, and a basic end-to-end suite of 10 tests.
     - The SQLite driver moved to modernc. modernc returns `DATETIME` columns as `time.Time` where ncruces returned the stored text, so every read now casts to text, and a test guards it.
     - Roslynator went, since the shared analyzers cover it.
     - Its API types, which tygo generates from the Go models, are now committed and checked by a small job of Senpan's own, as Kindred's are. So the shared web job needs no Go, and Senpan stays on v1.2.0. The move to huma comes next, on its own (decided 2026-11-06); it changes only the generator.
     - Its pre-push hook, which ran the whole gate before every push, is gone: CI is the gate, as in the other projects (decided 2026-11-06).
   - **Font Awesome bandwidth** (2026-11-07). The account went over its November bandwidth: CI installed from the Pro registry 50 times in the first week (13 Kindred, 21 Senpan, 15 TarotGen.io, 1 Dossier), since every changed lockfile missed setup-node's exact-key npm cache, and a job that failed after installing saved nothing. Senpan's kit held all 17 styles, a 201 MB download each time, about 4.2 GB of the 4.6 GB.
     - v2.1.5's shared workflows restore npm's download cache from the newest earlier run and save both caches straight after the install, so a package is fetched again only when its own version changes. Home Library's own CI took the same scheme.
     - The owner cut every kit to the styles its sites import (Senpan's to four, 42 MB), and the guide now says a kit holds only those.
     - The Gallery stopped loading its kit from `kit.fontawesome.com` on every page view: it installs the shared kit and renders inline SVG through `<font-awesome-icon>`, with a test that fails on an unregistered icon. Home Library moved from the per-style packages to the same kit.
     - Kits are not cut to chosen icons (Font Awesome's custom subsetting), decided 2026-11-07: it would make each package a few hundred KB, but every new icon would first need adding to the kit and a new package version. A few MB per style, fetched only when a kit's version changes, is the price of adding an icon freely.
   - **OSV and `task push`** (2026-11-07). Every project now runs the shared OSV job, which also gates its release: an `osv-scanner.toml` beside each npm lockfile passes over the three advisories that reach only lint, test and build tooling (minimatch through ESLint, glob through js-beautify and Workbox, deepmerge through Workbox), Kindred took `golang.org/x/net` 0.62.0, and Senpan's math rendering moved to KaTeX 0.19.1 by an override. `x/crypto`'s advisory waits for a tagged release; its code is uncalled, so it doesn't fail the scan. The next release adds `task shared:deps`, `shared:deps:update`, `shared:vuln` and `shared:push`; each project then gets `task push` and `shared:vuln` in its `task check`, so in-range updates are taken before a push rather than arriving as Dependabot's pull requests.
   - **Release tags from CI** (2026-11-07). The standard has CI tag each release, but Kindred, Marxbot and Home Library still tagged by hand or not at all; Kindred's standards pass had recorded it as a departure instead of adopting it, and its deploy then refused an untagged HEAD. All three now have the Dossier's `release` job and `release.sh`, reading `web/package.json` (Kindred 0.30.1, Home Library 1.1.0) or a `VERSION` file (Marxbot 1.0.0, which the bot embeds and logs), and each deploy fetches the tags and refuses a HEAD whose tag isn't that version. Kindred's hand tags had gone backwards (v0.29.5 and v0.29.6 after v0.30.0). The Gallery's, TarotGen.io's and the Dossier's deploys then took the same check (their CI already tagged releases; the Gallery hadn't recorded the gap as a departure), with `-Preview` for an untagged try. Senpan's waits for its move to one app version, since CI still tags its parts separately.
   - **Newest-release deploys** (2026-11-09). A deploy that required HEAD to carry the tag stopped as soon as a tooling update landed after a release (Home Library's v2.3.2 commit after v1.1.2), and checking the tag out by hand meant matching its submodule too. `task deploy` now ships the newest release CI tagged on main, built in its own checkout with clean installs from its lockfiles, and skips a release the host already runs (a record in `~/.shared-deploy/` on the host, written only by a deploy that passed its checks; `-Force` deploys it again). All six rolled-out projects moved to it with shared-tooling 2.4.0; Senpan takes it with its overhaul.
7. **Then the migrations, in this order:**
   1. Senpan's overhaul, as five releases, each shipped and deployed on its own (decided 2026-11-09). Its API moves to huma in C, replacing its hand-written spec and kin-openapi.
      - **A, infrastructure (4.0.0):** one app version with `v<version>` tags; the SPA embedded in the Go binary, with `/healthz`; Apache serving only the document root's own folders and proxying the rest to the app; shared-tooling 2.4.0 and its newest-release deploy; the binary owned by the service's own account. Deployed 2026-11-09.
      - **B, the data layer (4.1.0):** goose from a baseline at v71, a write pool and a read pool, RFC 3339 timestamps (production's drifted tables rebuilt to match a fresh build, checked on a copy of the live data first), sessions in SQLite under hashed tokens, and draws from crypto/rand (the owner took both, 2026-11-09). sqlc waits for C, domain by domain. Deployed 2026-11-09.
      - **Before C, a small front-end fix** (found 2026-11-09, npm's deprecation warning on glob 11.1.0): Workbox's build (through vite-plugin-pwa) requires glob ^11 and js-beautify (through @vue/test-utils) ^13, both at their newest releases, and the advisory GHSA-3g3m-2fjr-6pw4 is fixed only in glob 14.0.2. Both call only `globSync`, which glob 14 still ships for CommonJS, so an npm override of glob ^14.0.2 for each should hold; check that the PWA build's precache list is unchanged and the tests pass, then drop the advisory's entry from `frontend/osv-scanner.toml`. Done 2026-11-09: the service worker built byte for byte the same, the tests passed, and OSV-Scanner reports nothing.
      - **C, the API to huma (5.0.0):** domain packages with thin handlers, RFC 9457 errors, 422 for validation, camelCase JSON with openapi-typescript, and the plugin's API models moving with it. Decided 2026-11-09: the plugin moves in the same release and deploys the same day, no endpoint keeps snake_case, and the public tea-room API changes too (the Carrd pages' scripts with it); the live feed moves from WebSockets to SSE, as Kindred's, since its traffic is one-way; 502s give way to the standard's 503/500/429, public reads take the caching baseline, and the tea-room API's `*` gives way to an allowlist setting. Its plan, domain by domain, is Senpan's `plans/release-c.md`.
      - **D, front-end cleanup:** specs in `src/__tests__/` with the config in `vite.config.ts`, and the static inline styles into CSS so the shared rule comes back on. It touches no backend, so it can come at any point.
      - **E, nothing served from `/var/www`, the goal (decided 2026-11-09):** the app serves everything, as Home Library and Kindred Nights do, with its vhost proxying `/` to it and its data under `/opt/senpan`. The uploads (`images/`, `fonts/`), the Carrd sites and the plugin repository leave the document root: the app serves `/images/`, `/plugin/` (as Kindred serves its own) and the Carrd sites, the `carrd.senpan.cafe` vhost proxies to it, `-webroot` gives way to a data folder, and the deploy ships the vhost, as Home Library's does. It comes after C, so the file serving is written once, into C's domain packages. The fonts need the most care. They are licensed, so they must not be reachable as plain files at any point in the move. `fonts.senpan.cafe/kit.css` and its token URLs, which external Carrd sites embed, must stay as they are through C and E. The live `fonts.senpan.cafe` vhost is read first, since the repo can't see whether it still names the folder. The folder moves whole, with its hidden `.woff2/` copies, while the service is stopped, and is checked afterwards from an allowed site and from Safari, which sends a `Referer` in place of an `Origin`.
   2. Sign-in inside huma in Home Library, then Kindred Nights (decided 2026-11-09, as Senpan's release C does it). Both sign in and out through plain handlers under `/auth/*`, outside huma and the API document; huma doesn't ask for that, and the standard puts these endpoints inside the API. They become operations under `/api/v1/auth/*`, called through the typed client, as Senpan's are. Kindred's Discord start and callback become operations that answer 302. Home Library's share links and calendar feed stay outside, being addresses already handed out. The standard's line 24 then names `/api/v1/auth/*` and gains a rule for operations a browser navigates to. The plan, project by project, is this repo's `plans/sign-in-on-huma.md`. It also covers what adding Discord sign-in to Senpan or Home Library later would take: it argues for an identities table, not for keeping sign-in outside huma.

<!-- markdownlint-enable MD029 -->

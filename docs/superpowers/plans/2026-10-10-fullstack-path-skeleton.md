# Full-Stack Engineer Path: Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Astro Starlight site, its diagram pipeline, components, page stubs and GitHub Pages deploy, plus one fully written sample week (Week 4, Networking), as three reviewable PRs.

**Architecture:** A static Astro Starlight site. Diagrams are written in D2 and rendered at build time to inline SVG themed by Starlight CSS variables, with optional step-through walkthroughs. Week and track pages are MDX in Starlight's docs collection with an extended frontmatter schema. A small set of Astro components provides the week template pieces, and a tested `examples/` tree feeds code snippets into pages. One GitHub Actions workflow builds, tests and deploys to Pages.

**Tech Stack:** Astro 7 + `@astrojs/starlight` 0.42, `d2` CLI v0.7.1 (called at build time from a `<Diagram>` component), `starlight-links-validator`, Node 22, `node:test`, `@playwright/test`, Go (current stable), Vitest, GitHub Actions (`actions/configure-pages`, `actions/upload-pages-artifact`, `actions/deploy-pages`).

**Spec:** Claude Doc "Full-Stack Engineer Path: Design Spec" (https://claude.ai/code/artifact/079a6f10-cf4a-4d7d-b0ac-4ac987406d99). Task 4 commits it to `docs/superpowers/specs/2026-10-10-fullstack-path-design.md`.

## Global Constraints

- Reader: QA/frontend engineer moving to full-stack; backend and ops weighted heavier.
- Pace: 5 to 8 hours per week; weeks 0 to 18 (19 weeks).
- Backend examples in Go; frontend examples in TypeScript and React.
- Layout: Starlight's left nav, center content, right "On this page" TOC. No custom layout.
- Diagrams (revised 2026-10-10 by Casey's decision): authored as D2 text in a `<Diagram>` component, rendered at build time to INLINE SVG whose colors are CSS variables mapped to Starlight's theme (follows the `data-theme` toggle). Optional walkthrough: Prev/Next steps highlight named shapes/edges, dim the rest, show a caption; without JS the steps render as an ordered list. Only the walkthrough script (< 2 KB) ships to readers. Mermaid dropped (controller ruling, see ledger).
- Hosting: GitHub Pages via GitHub Actions. The site lives under a repo sub-path (`/<repo>/`); every link and asset must respect `base`.
- Progress state lives only in the reader's browser (`localStorage`).
- AI ladder: stage 1 weeks 0 to 5, stage 2 weeks 6 to 12, stage 3 weeks 13 to 18.
- Each PR passes CI before review: `go test`, Vitest, node build tests, Playwright, link validation.
- No reference implementation repo; no custom domain; no provider-specific cloud tracks.

## Review Focus

1. **Sub-path hosting.** Site served at `https://<user>.github.io/<repo>/`; every internal link, image and component link must work there, not just at `/`. Pinned by Task 1's `base` test and Task 4's build-with-base step.
2. **`localStorage` unavailable** (private window, blocked storage). `DoneWhen` and `WeekTimeline` must render and not throw; ticks just don't persist. Pinned in Task 7.
3. **Theme toggle on diagrams.** Light OS + Starlight dark (and the reverse) must still give readable diagrams, because colors come from `data-theme`, not `prefers-color-scheme`. Pinned in Task 3's Playwright test (computed fill differs between themes).
4. **Typo in frontmatter** (unknown track slug, week out of range, missing field). Build must fail with a message naming the file. Pinned in Task 5.
5. **Phone width** (375px). Wide diagrams and tables scroll inside their own box; the page itself never scrolls horizontally. Pinned in Task 7's Playwright test.

---

## File Structure

```text
.
├── package.json                      # scripts: dev, build, preview, test:build, test:e2e
├── astro.config.mjs                  # Starlight, sidebar, base/site, links validator
├── src/
│   ├── content.config.ts             # docs collection + extended schema
│   ├── data/tracks.ts                # the 25 tracks: slug, title, group
│   ├── data/weeks.ts                 # the 19 weeks: number, slug, title, phase
│   ├── lib/links.ts                  # weeksForTrack(), trackHref(), weekHref()
│   ├── lib/progress.ts               # localStorage helpers (safe when storage throws)
│   ├── components/
│   │   ├── WeekHeader.astro
│   │   ├── ObserveIt.astro  OperateIt.astro  AiThisWeek.astro  SideLab.astro
│   │   ├── DoneWhen.astro
│   │   ├── ReadWith.astro  UsedInWeeks.astro
│   │   ├── WeekTimeline.astro
│   │   └── CodeFrom.astro
│   ├── styles/custom.css             # callout colors, overflow rules
│   └── content/docs/
│       ├── index.mdx  start-here.mdx
│       ├── weeks/00-setup.mdx … 18-sre.mdx
│       └── tracks/<group>/<slug>.mdx
├── examples/
│   ├── go/  (go.mod, week04/trace.go, week04/trace_test.go)
│   └── web/ (package.json, vitest config, placeholder test)
├── tests/
│   ├── build/*.test.mjs              # node:test assertions over dist/
│   └── e2e/*.spec.ts                 # Playwright against astro preview
├── docs/superpowers/specs/2026-10-10-fullstack-path-design.md
├── docs/superpowers/plans/2026-10-10-fullstack-path-skeleton.md
└── .github/workflows/site.yml
```

**Week slugs** (`src/data/weeks.ts`, phase in brackets): `00-setup` [foundations], `01-go-basics`, `02-go-packages-testing`, `03-linux-processes`, `04-networking`, `05-concurrency`, `06-architecture-repo` [build], `07-backend-api`, `08-backend-postgres`, `09-jobs-alerts`, `10-frontend`, `11-security-auth` [harden], `12-debugging`, `13-observability`, `14-ci-cd`, `15-kubernetes-local` [run], `16-going-live`, `17-day-2-traffic`, `18-sre`.

**Track slugs** (`src/data/tracks.ts`, by group):
- `foundations`: `programming`, `operating-systems`, `networking`, `dns`, `time-ntp`, `storage`
- `building`: `databases`, `messaging`, `system-design`, `testing`
- `shipping`: `ci-cd`, `automation`, `containers`, `kubernetes`
- `running`: `observability`, `load-balancing`, `backups`, `tls-pki`, `iam`, `security`, `sre`
- `working`: `ai-assisted-dev`, `git`, `documentation`, `work-tracking`

---

# PR 1: Site and deploy (branch `skeleton/site-deploy`)

### Task 1: Starlight project with base-path-aware build tests

**Files:**
- Create: `package.json`, `astro.config.mjs`, `tsconfig.json`, `src/content/docs/index.mdx`, `src/content.config.ts` (stock `docsSchema()` for now), `tests/build/base.test.mjs`, `.gitignore`

**Interfaces:**
- Produces: npm scripts `build`, `preview`, `test:build` (`node --test tests/build`). `astro.config.mjs` reads `process.env.SITE_URL` (default `http://localhost:4321`) and `process.env.BASE_PATH` (default `/`).

- [ ] **Step 1: Write the failing test** `tests/build/base.test.mjs`: reads `dist/index.html`; asserts it contains `<title>` with "Full-Stack Engineer Path"; when `BASE_PATH=/fsp/`, asserts every `href="/` and `src="/` in the file starts with `/fsp/`.
- [ ] **Step 2: Run** `npm run test:build` → FAIL (no `dist/`).
- [ ] **Step 3: Scaffold** with `npm create astro@latest -- --template starlight`, set title "Full-Stack Engineer Path", wire `site`/`base` from env, minimal home page.
- [ ] **Step 4: Run** `npm run build && npm run test:build` and `BASE_PATH=/fsp/ npm run build && BASE_PATH=/fsp/ npm run test:build` → both PASS.
- [ ] **Step 5: Commit** `feat: scaffold Starlight site with base-path-aware build`.

### Task 2: D2 renderer and `<Diagram>` component (inline, themed SVG)

Replaces the earlier Mermaid work: remove `rehype-mermaid`, `@astrojs/markdown-remark` config, `src/plugins/rehype-diagram-themes.mjs`, `markdown.syntaxHighlight.excludeLangs`, the Playwright launch override in `astro.config.mjs`, and `tests/build/mermaid.test.mjs`. Keep `playwright` only if Task 3 needs it as a devDependency for e2e.

**Files:**
- Create: `src/lib/diagram/render.mjs`, `src/lib/diagram/keys.mjs`, `src/components/Diagram.astro`, `src/styles/diagrams.css` (replace contents), `tests/unit/diagram.test.mjs`, `tests/build/diagram.test.mjs`
- Modify: `astro.config.mjs`, `package.json` (script `test:unit` = `node --test "tests/unit/*.test.mjs"`), `src/content/docs/dev/diagrams.mdx` (fixture: one D2 flowchart `browser -> resolver -> root; resolver -> tld: cache miss; browser -> server` and one D2 `shape: sequence_diagram` with `browser -> server: SYN`, `server -> browser: SYN-ACK`, `browser -> server: ACK`)

**Interfaces:**
- Produces:
  - `renderD2(source: string, opts: { id: string }): Promise<{ svg: string, keys: Set<string> }>` in `render.mjs`. Runs `d2 --no-xml-tag --omit-version --pad 0 --salt <id> - -` (stdin → stdout), throws with the d2 stderr on failure. Post-processes: rewrites every `.d2-<hash> .fill-XX{fill:#...}` / `.stroke-XX` / `.color-XX` rule to `var(--d2-XX)`; sets `width`/`height` from the viewBox; adds `data-k="<normalized key>"` to each shape/edge group; prefixes any remaining ids with `<id>-`. Results cached in memory by sha256(source + d2 version + id).
  - `decodeKey(className: string): string | null` and normalization in `keys.mjs`: D2 class = base64 of the HTML-escaped key. Normalized forms: shape `browser`, nested `vpc.api`, edge `browser->resolver` (index 0 implied), `browser->resolver#1` for the second edge between the same pair, sequence messages `browser->server`, `server->browser`, `browser->server#1` in order.
  - `<Diagram id="dns-lookup" title="…" code={`…d2…`} steps?={[{ hl: string[], caption: string }]} />`: renders `<figure class="dg" id={id}>` with the inline SVG, `<figcaption>` = title, and when `steps` is given an `<ol class="dg-steps">` with `<li data-hl="k1 k2">caption</li>` per step. Throws at build if a `hl` key is not in `keys` (message names the diagram id and the key), or if `id` repeats on a page.
  - `diagrams.css`: maps every `--d2-*` used by D2's default theme (N1–N7, B1–B6, AA2/AA4/AA5, AB4/AB5) to Starlight vars (`--sl-color-text`, `--sl-color-gray-1..6`, `--sl-color-accent`, `--sl-color-accent-low`, `--sl-color-bg`, …) once; no light/dark duplication needed because Starlight's vars already switch with `data-theme`. Also `.dg svg { max-width: 100%; height: auto }` inside an `overflow-x: auto` wrapper.

- [ ] **Step 1: Write failing unit tests** `tests/unit/diagram.test.mjs`: `decodeKey` round-trips `browser`, `vpc.api`, `(browser -&gt; dns)[0]` → `browser->dns`, `(b -&gt; s)[1]` → `b->s#1`; `renderD2` on the flowchart fixture returns `keys` containing `browser`, `resolver`, `browser->resolver`, and an svg string with no `#` hex inside any `.fill-`/`.stroke-` rule, containing `var(--d2-`, `data-k="browser"`, explicit `width=`; rendering the same source with ids `a` and `b` yields no shared element ids; invalid D2 (`a -> {`) rejects with an error mentioning the d2 message.
- [ ] **Step 2: Write failing build test** `tests/build/diagram.test.mjs`: fixture page has two `<figure class="dg"` each containing an inline `<svg`, no `<img` inside figures, no `<picture>`, an `<ol class="dg-steps">` with `data-hl` on the flowchart; built CSS contains `--d2-B1` mapped to a `var(--sl-color-`; a temp fixture with `steps={[{hl:['nope'],caption:'x'}]}` makes `npm run build` exit non-zero naming `nope` (copy, build, delete in `finally`).
- [ ] **Step 3: Run** `npm run test:unit` and `npm run build && npm run test:build` → FAIL.
- [ ] **Step 4: Implement** per Interfaces. Parse SVG with a real parser (`hast-util-from-html` / `unified` + `rehype-parse` are fine) rather than regexes for the `data-k` and id-prefix steps; regex is fine for the CSS rule rewrite.
- [ ] **Step 5: Run** unit + build tests on default and `BASE_PATH=/fsp/` builds → PASS. Commit `feat: build-time themed D2 diagrams via Diagram component`.

### Task 3: Walkthrough script and theme e2e test

**Files:**
- Create: `src/scripts/walkthrough.ts` (loaded by `Diagram.astro` via a `<script>` tag so Astro bundles it once), `playwright.config.ts` (webServer `npm run preview`, baseURL from `BASE_PATH`), `tests/e2e/diagram.spec.ts`
- Modify: `package.json` (devDependency `@playwright/test`; script `test:e2e`), `src/styles/diagrams.css` (`.dg-dim { opacity: .25 }`, `.dg-hl` stroke `var(--sl-color-accent)` and stroke-width 2 on shapes/edges)

**Interfaces:**
- Consumes: `<figure class="dg">`, `<ol class="dg-steps"><li data-hl="…">` and `data-k` attributes from Task 2.
- Produces: for each figure with steps, the script hides the `<ol>`, inserts a control row `<div class="dg-nav"><button>Prev</button><span aria-live="polite">1/N: caption</span><button>Next</button></div>`, and on each step adds `dg-hl` to elements whose `data-k` is in the step's list and `dg-dim` to all other `[data-k]`. Buttons disable at the ends. Arrow keys work when the nav has focus. No JS → the `<ol>` stays visible and the diagram undimmed.

- [ ] **Step 1: Write failing e2e tests** `diagram.spec.ts` on the fixture page: (a) step 1 shows caption "1/" and the first step's keys have class `dg-hl`, others `dg-dim`; Next → "2/", highlights move; Prev at step 1 is disabled; (b) with `javaScriptEnabled: false`, the `ol.dg-steps` is visible and no element has `dg-dim`; (c) theme: set `localStorage['starlight-theme']='dark'` with `colorScheme: 'light'` and read the computed `fill` of `[data-k="browser"] rect` (or the shape's first path); set `'light'` with `colorScheme: 'dark'` and read again; the two fills differ and each matches the page's resolved theme (dark fill darker than light fill by luminance).
- [ ] **Step 2: Run** `npm run build && npm run test:e2e` → FAIL. (Locally use `PLAYWRIGHT_CHROMIUM_EXECUTABLE=/opt/pw-browsers/chromium-1194/chrome-linux/chrome` via `launchOptions.executablePath` only when that env var is set.)
- [ ] **Step 3: Implement** the script and CSS.
- [ ] **Step 4: Run** e2e on default and `BASE_PATH=/fsp/` → PASS; script bundle < 2 KB minified (check `dist/_astro/*.js` size for it).
- [ ] **Step 5: Commit** `feat: diagram walkthrough steps and theme e2e tests`.

### Task 4: CI, Pages deploy, link validation, spec committed

**Files:**
- Create: `.github/workflows/site.yml`, `docs/superpowers/specs/2026-10-10-fullstack-path-design.md` (exported from the Claude Doc), `docs/superpowers/plans/2026-10-10-fullstack-path-skeleton.md` (this file), `README.md` (one paragraph + local dev commands)
- Modify: `astro.config.mjs` (add `starlight-links-validator`)

- [ ] **Step 1: Prove the validator bites**: add a link to `/does-not-exist/` in `index.mdx`, run `npm run build` → FAIL naming the link. Remove it → PASS.
- [ ] **Step 2: Write `site.yml`**: triggers `pull_request` and `push` to `main`; `permissions: contents: read, pages: write, id-token: write`; `concurrency: { group: pages, cancel-in-progress: false }`. Build job: checkout, setup-node 22 with npm cache, `actions/configure-pages` (its `base_path` output → `BASE_PATH`, its `base_url` → `SITE_URL`), install `d2` v0.7.1 from its GitHub release tarball into `/usr/local/bin`, `npm ci`, `npm run test:unit`, `npm run build`, `npm run test:build`, `npx playwright install --with-deps chromium`, `npm run test:e2e`, `actions/upload-pages-artifact` with `path: dist`. On PRs also upload `dist` with `actions/upload-artifact` for review. Deploy job: `if: github.ref == 'refs/heads/main'`, `needs: build`, environment `github-pages`, `actions/deploy-pages`.
- [ ] **Step 3: Verify** with `actionlint .github/workflows/site.yml` → no errors.
- [ ] **Step 4: Commit** `ci: build, test and deploy to GitHub Pages; add design spec and plan`.
- [ ] **Step 5: Open PR 1**, confirm CI green, ask Casey to set Pages source to "GitHub Actions" in repo settings, and after merge confirm the live URL loads the home page and the diagram fixture.

---

# PR 2: Components and stubs (branch `skeleton/components`)

### Task 5: Content schema, week/track registries, stub pages, sidebar

**Files:**
- Create: `src/data/tracks.ts`, `src/data/weeks.ts`, 19 week stubs, 25 track stubs, `src/content/docs/start-here.mdx`, `tests/build/schema.test.mjs`
- Modify: `src/content.config.ts`, `astro.config.mjs` (sidebar)

**Interfaces:**
- Produces:
  - `type TrackGroup = 'foundations' | 'building' | 'shipping' | 'running' | 'working'`
  - `type Track = { slug: TrackSlug; title: string; group: TrackGroup }`; `export const tracks: readonly Track[]`; `export type TrackSlug` (union of the 25 slugs).
  - `type Phase = 'foundations' | 'build' | 'harden' | 'run'`; `type Week = { n: number; slug: string; title: string; phase: Phase }`; `export const weeks: readonly Week[]`.
  - Week frontmatter (extended `docsSchema`, optional for non-week pages, validated when `week` is present): `week: number (0..18)`, `phase: Phase`, `hours: [number, number]`, `aiStage: 1 | 2 | 3`, `tracks: TrackSlug[]` (min 1).
  - Track frontmatter: `track: TrackSlug`.
- Sidebar: Start here; Weeks in four groups by phase (labels "Foundations (0–5)", "Build the app (6–10)", "Harden it (11–14)", "Run it for real (15–18)"); Tracks in five groups by `TrackGroup`.

- [ ] **Step 1: Write failing tests** `schema.test.mjs`: (a) build succeeds and `dist/weeks/` holds 19 pages and `dist/tracks/` 25; (b) copying a fixture week with `tracks: [nope]` into `src/content/docs/weeks/` makes `npm run build` exit non-zero with output containing the file name and `nope` (test copies, builds, deletes in `finally`); (c) `aiStage` in each stub matches the ladder (weeks 0–5 → 1, 6–12 → 2, 13–18 → 3).
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** registries, schema (`zod` enum from the registries), stubs (frontmatter filled from the spec's week table; week body = the week template's section headings with "Coming soon"; track body = the track template's headings: What it is, Diagrams, Example, In the uptime monitor, Common mistakes, Going deeper, Further reading), and the sidebar generated from the registries.
- [ ] **Step 4: Run** build + tests → PASS.
- [ ] **Step 5: Commit** `feat: content schema, registries and stub pages`.

### Task 6: Week template components and week↔track links

**Files:**
- Create: `WeekHeader.astro`, `ObserveIt.astro`, `OperateIt.astro`, `AiThisWeek.astro`, `SideLab.astro`, `ReadWith.astro`, `UsedInWeeks.astro`, `src/lib/links.ts`, `src/styles/custom.css`, `tests/build/links.test.mjs`
- Modify: all stubs to use the components

**Interfaces:**
- Produces:
  - `weeksForTrack(slug: TrackSlug): Promise<number[]>` (sorted ascending, from `getCollection('docs')` week frontmatter).
  - `weekHref(n: number): string`, `trackHref(slug: TrackSlug): string`, both prefixed with `import.meta.env.BASE_URL`.
  - `<WeekHeader />` reads the current page's frontmatter (`Astro.locals.starlightRoute`) and shows phase, hours as "5–8 h", AI stage name, prerequisites link to previous week.
  - `<AiThisWeek>` shows the stage name ("Explain only", "Review everything", "Pair partner") from frontmatter plus its slot.
  - `<SideLab title="…">` renders a `<details>`.
  - `<ReadWith />` and `<UsedInWeeks />` take no props; read frontmatter.

- [ ] **Step 1: Write failing tests** `links.test.mjs`: `dist/tracks/foundations/networking/index.html` contains "Used in weeks" with links to weeks 4, 12 (from the spec's table); `dist/weeks/04-networking/index.html` contains links to the networking, dns, tls-pki and time-ntp tracks; with `BASE_PATH=/fsp/` those hrefs start with `/fsp/`.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** components and `links.ts`; callouts styled with Starlight CSS variables so dark mode works.
- [ ] **Step 4: Run** build + tests (both base paths) → PASS.
- [ ] **Step 5: Commit** `feat: week template components and week-track backlinks`.

### Task 7: DoneWhen, WeekTimeline, progress storage

**Files:**
- Create: `src/lib/progress.ts`, `DoneWhen.astro`, `WeekTimeline.astro`, `tests/e2e/progress.spec.ts`, `tests/e2e/mobile.spec.ts`
- Modify: `src/content/docs/index.mdx` (add a short "how to use this", the timeline, and a "Start at week 0" link button), week stubs (add `<DoneWhen>` with 3 placeholder items)

**Interfaces:**
- Produces:
  - Storage key per week: `fsp:done:week-NN` (two-digit), value: JSON array of checked item ids.
  - `loadDone(week: number): string[]`, `saveDone(week: number, ids: string[]): void`, `weekComplete(week: number, total: number): boolean`; all wrap storage access in try/catch and fall back to an in-memory map.
  - `<DoneWhen items={[{ id: string; text: string }]} />` renders checkboxes, persists on change, and writes `data-total` for the timeline.
  - `<WeekTimeline />` renders 19 week nodes grouped by phase, each linking to its week, marked complete when `weekComplete` is true (client script).
  - Reuses Task 3's `playwright.config.ts` and `test:e2e`.

- [ ] **Step 1: Write failing tests** `progress.spec.ts`: tick 3 of 3 on week 4, reload → still ticked; home page timeline marks week 4 complete; with `localStorage` access stubbed to throw (`page.addInitScript` overriding `Storage.prototype.getItem/setItem`), week 4 renders, ticking works for the session, no page error is logged. `mobile.spec.ts`: at 375×800 on the diagram fixture and week 4, `document.documentElement.scrollWidth <= 375`.
- [ ] **Step 2: Run** `npm run test:e2e` → FAIL.
- [ ] **Step 3: Implement** `progress.ts`, components, and CSS: `.sl-markdown-content svg, table` wrappers get `overflow-x: auto`.
- [ ] **Step 4: Run** `npm run test:e2e` → PASS.
- [ ] **Step 5: Commit** `feat: done-when checklists and progress timeline`.

### Task 8: Tested examples tree and CodeFrom

**Files:**
- Create: `examples/go/go.mod` (module `fsp/examples`), `examples/go/week04/trace.go`, `examples/go/week04/trace_test.go`, `examples/web/package.json`, `examples/web/vitest.config.ts`, `examples/web/src/sanity.test.ts`, `src/components/CodeFrom.astro`, `tests/build/codefrom.test.mjs`
- Modify: `site.yml`, `src/content/docs/dev/diagrams.mdx` (use `<CodeFrom>` once)

**Interfaces:**
- Produces:
  - Region markers in example files: `// region: <name>` … `// endregion: <name>`.
  - `<CodeFrom file="go/week04/trace.go" region="timings" lang="go" title?="…" />` reads the file at build time relative to `examples/`, extracts the region, renders through Starlight's `<Code>`; missing file or region throws at build with the file and region named.
  - `examples/go/week04`: `type Timings struct { DNS, Connect, TLS, FirstByte, Total time.Duration }`; `func Check(ctx context.Context, url string) (Timings, int, error)` using `net/http/httptrace`.

- [ ] **Step 1: Write failing tests**: `trace_test.go` → `TestCheckRecordsTimings` against an `httptest.NewTLSServer` (client trusts its cert): status 200, `TLS > 0`, `FirstByte > 0`, `Total >= FirstByte`; `TestCheckHonorsContext` with a 1 ms timeout against a handler that sleeps 50 ms → error is `context.DeadlineExceeded` (via `errors.Is`). `codefrom.test.mjs`: the fixture page contains the text `httptrace.ClientTrace`; a fixture with `region="missing"` fails the build with "missing" in output.
- [ ] **Step 2: Run** `cd examples/go && go test ./...` and `npm run test:build` → FAIL.
- [ ] **Step 3: Implement** `Check` and `CodeFrom`. DNS timing is 0 for an IP-literal test server; don't assert it.
- [ ] **Step 4: Run** `go vet ./... && go test -race ./...`, `cd examples/web && npx vitest run`, build tests → PASS.
- [ ] **Step 5: Add** setup-go (version from `examples/go/go.mod`), `go vet`, `go test -race`, and Vitest steps to `site.yml`. **Commit** `feat: tested examples tree and CodeFrom component`. **Open PR 2.**

---

# PR 3: Sample week (branch `content/week-04-networking`)

### Task 9: Week 4, Networking, fully written

**Files:**
- Modify: `src/content/docs/weeks/04-networking.mdx`, `examples/go/week04/*`
- Create: `examples/go/week04/cmd/check/main.go` (CLI printing the timing table), `tests/build/week04.test.mjs`

**Content requirements (from the spec's week template):**
- `WeekHeader`; Goals (3 to 5 bullets); Concepts with `<Diagram>`s: a D2 sequence diagram of DNS resolution (stub resolver → recursive → root → TLD → authoritative) with a walkthrough of each hop, a D2 sequence diagram of the TCP handshake then TLS 1.3 handshake with a walkthrough, and a D2 diagram of the request path (CLI → resolver → network → load balancer → server); Build: steps extending the uptime monitor CLI to print DNS/connect/TLS/first-byte, using `<CodeFrom>` regions from `trace.go` and `main.go`; `ObserveIt` (log the four timings per check, spot DNS vs TLS slowness); `OperateIt` (what an expired cert, a DNS NXDOMAIN, and a black-holed port each look like in the output); `AiThisWeek` (stage 1: ask it to explain a `dig +trace` output, don't ask it to write `httptrace` code); `SideLab` ("Trace a lookup with `dig +trace` and compare with your CLI's DNS time"); `DoneWhen` with 4 to 6 concrete items; `ReadWith`.
- Pitch: assumes Weeks 1 to 3 done; explain terms on first use; no unexplained acronyms.

- [ ] **Step 1: Write failing test** `week04.test.mjs`: page has ≥3 `figure.dg` inline SVGs, ≥2 with `ol.dg-steps`, ≥2 Go code blocks containing `httptrace`, the `ObserveIt`, `OperateIt`, `AiThisWeek`, `SideLab` and `DoneWhen` markers, and no "Coming soon".
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Write** `main.go` (flags `-url`, `-timeout` default 10s; prints a 5-row table) with a test for its output format, then the page content.
- [ ] **Step 4: Run** all of: `go test -race ./...`, `npm run build`, `npm run test:build`, `npm run test:e2e` → PASS. Read the rendered page at 375px and desktop, light and dark.
- [ ] **Step 5: Commit** `content: week 4 networking sample week`. **Open PR 3** and ask Casey to review tone and depth before any other week is written.

---

## After this plan

Content PRs follow the spec's build plan (one week per PR, two or three tracks per PR) and get their own short plans once Week 4's style is approved.

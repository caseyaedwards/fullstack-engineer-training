# Full-Stack Engineer Path: Design Spec

2026-10-10 · Casey Edwards · Source of truth: the Claude Doc this file was exported from; approved 2026-10-10.

## Purpose, audience and success

This is a self-paced, 19-week website that takes an engineer from QA and frontend work to confident full-stack work, with backend and operations as the main focus.

**Reader.** A friend of Casey's who knows TypeScript and React basics and has a strong testing background, but is new to Go, Linux, networking and running services. Frontend weeks are lighter; backend and ops weeks go deeper. Their QA strength is used throughout (tests, debugging, CI).

**Pace.** Sized for 5 to 8 hours a week, about 4 to 5 months. Every week shows a time estimate and a "done when" checklist, so the reader can stretch or compress weeks without losing track.

**Success looks like:**

- The reader finishes with a working uptime monitor they built, deployed on a real domain with TLS, observed with metrics and traces, and covered by an SLO.
- They can explain each layer (OS, network, backend, frontend, deploy, operations) with the diagrams from the site.
- They use AI tools deliberately, having learned the fundamentals without leaning on them first.
- Casey can share one link and the site stands on its own.

## Decisions

All decisions below were agreed in the project thread on 2026-10-10.

| Area | Decision |
| --- | --- |
| Structure | One running project across all weeks, plus optional side labs per week |
| Running project | Uptime monitor and status page |
| Pace | 5 to 8 hours a week, self-paced, with a suggested timeline |
| Sequence | 19 weeks (0 to 18), reordered from the original 0 to 14 list |
| Languages | Go for backend examples, TypeScript and React for frontend |
| Reader environment | Laptop first (Docker Compose, kind), then a small cloud setup (about $10 to $25 a month) for weeks 16 to 18 |
| Site framework | Astro Starlight (left nav, content, right on-page table of contents) |
| Diagrams | Written as D2 text, rendered at build time to inline SVG themed by the site's CSS, with optional step-by-step walkthroughs (revised 2026-10-10) |
| Hosting | GitHub Pages via a GitHub Actions workflow, public repo `fullstack-engineer-training` |
| Build order | Skeleton plus one sample week first, then small PRs reviewed one at a time |
| Spec location | This file |

## The running project: uptime monitor

The reader builds one app across all 19 weeks: users add URLs, Go workers check them on a schedule, and a React status page shows current status, history and alerts.

It starts as a single-file Go CLI in week 1 and ends as a set of services on Kubernetes behind a real domain. It was chosen because nearly every week and track has a natural hook in it:

- **Concurrency:** checking hundreds of URLs in parallel with timeouts and cancellation.
- **Networking, DNS, TLS:** each check records DNS, connect, TLS handshake and first-byte timing, and flags certificates near expiry.
- **Databases and storage:** check history, retention and indexes on time-series-shaped data.
- **Messaging:** check results and alerts flow through a queue.
- **Observability and SRE:** the app monitors other sites and is itself monitored, with an SLO on its own status page.

**Components at the end of week 18:**

| Component | Language | Role |
| --- | --- | --- |
| `api` | Go | HTTP API for monitors, results, users; serves auth |
| `scheduler` | Go | Decides which checks are due and enqueues them |
| `worker` | Go | Runs checks with timing breakdown, writes results |
| `alerter` | Go | Consumes failures, applies rules, sends notifications |
| `web` | TypeScript, React | Status page and admin UI |
| Postgres | | Monitors, users, check history |
| Queue | | Check jobs and result events (choice made in week 9) |
| Prometheus, Grafana, OTel collector | | Metrics, dashboards, traces |

```d2
web -> api: JSON
api -> postgres: SQL
scheduler -> postgres: reads due monitors
scheduler -> queue: jobs
queue -> worker: jobs
worker -> queue: results
worker -> sites: checks
worker -> postgres: writes check results
queue -> alerter: failures
alerter -> notifications: sends
```

The worker is the week-1 CLI grown up; Prometheus scrapes every Go service and traces go to the OTel collector (not drawn).

Code for each step lives in the site's `examples/` folder as compiled, tested snippets. A complete reference repo is out of scope for now.

## Week-by-week path

Nineteen weeks in four phases, each week adding one capability to the uptime monitor; weeks 0 to 5 are foundations, 6 to 10 build the app, 11 to 14 harden it, 15 to 18 run it for real.

| Week | Title | Uptime monitor milestone | Read with this week |
| --- | --- | --- | --- |
| 0 | Setup | Toolchain installed, repo created, AI-use rules read | Git, AI-assisted development, ticket tracking |
| 1 | Go basics | CLI checks one URL and prints status and latency | Core programming concepts |
| 2 | Go: packages, interfaces, testing | Checks a list from a file, table tests | Core programming concepts, testing |
| 3 | Linux and processes | Long-running process, handles SIGTERM, logs to stdout | Operating systems, storage |
| 4 | Networking | Each check records DNS, connect, TLS and first-byte time | Networking, DNS, TLS and PKI, time and NTP |
| 5 | Concurrency | Worker pool checks 100 URLs with timeouts and context | Core programming concepts, operating systems |
| 6 | Frontend vs backend, repo skeleton | Monorepo layout, Makefile, Docker Compose | System design, containers, documentation |
| 7 | Backend: HTTP API | REST API for monitors, JSON, validation, OpenAPI | System design, testing |
| 8 | Backend: Postgres | Schema, migrations, indexes, check history | Databases, storage, backups |
| 9 | Background jobs and alerts | Scheduler, queue, alerter sends notifications | Messaging and streaming, system design |
| 10 | Frontend | React status page with history charts, Vitest, Playwright | Testing, system design |
| 11 | Security and auth | Login, sessions vs tokens, secrets, OWASP fixes | Security, IAM, TLS and PKI |
| 12 | Debugging | Find a planted bug with delve, pprof, curl, devtools | Operating systems, networking, observability |
| 13 | Observability | slog, Prometheus metrics, OTel traces, Grafana | Observability |
| 14 | CI/CD | GitHub Actions: test, lint, build images, deploy | CI/CD, automation, Git |
| 15 | Kubernetes on the laptop | kind cluster, deployments, services, probes, config | Kubernetes, containers |
| 16 | Going live | Cloud box, real DNS, Let's Encrypt, ingress, load balancer | DNS, TLS and PKI, load balancing, IAM |
| 17 | Day 2 and traffic | Scaling, rollouts and rollback, rate limits, restore drill | Load balancing, backups, automation |
| 18 | SRE | SLOs and error budgets, on-call, incident drill, postmortem | SRE and incident response, observability |

**Observability and SRE are woven into every week** through three standard boxes, so week 13 and week 18 deepen habits rather than introduce them:

- **Observe it:** what to log or measure in what was just built (week 1: print latency; week 5: count timeouts; week 8: slow-query log).
- **Operate it:** what breaks and how you'd know (week 3: what happens on SIGKILL; week 8: disk fills up).
- **AI this week:** the current AI-ladder stage and one concrete do and don't.

Each week also has an optional **side lab** for concepts that don't fit the app, such as using `dig` to trace DNS resolution or watching clock drift with `chronyc`.

## Topic tracks

Twenty-five reference tracks in five groups sit beside the weekly path; the reader can open any of them at any time.

| Group | Tracks |
| --- | --- |
| Foundations | Core programming concepts, operating systems, networking, DNS, time and NTP, storage |
| Building | Databases, messaging and streaming, system and software design, testing |
| Shipping | CI/CD, automation (including Ansible), containers, Kubernetes |
| Running | Observability, load balancing, backups, TLS and PKI, IAM, security, SRE and incident response |
| Working | AI-assisted development, Git, documentation, ticket and work tracking |

Testing, containers, security, SRE and incident response, and Git were added during brainstorming.

**Track page layout**, in this order:

1. What it is and why it matters
2. Diagrams
3. A minimal Go or TypeScript example
4. In the uptime monitor: where this shows up in the project
5. Common mistakes
6. Going deeper
7. Further reading

**Links between weeks and tracks.** Each week's frontmatter lists its tracks, which render as "Read with this week". Each track page renders "Used in weeks N, M" from that same frontmatter at build time, so the links cannot drift.

## AI-assisted development ladder

AI use loosens in three stages, so the reader builds the fundamentals by hand before handing work to a tool.

| Stage | Weeks | Allowed | Not allowed |
| --- | --- | --- | --- |
| 1. Explain only | 0 to 5 | Ask it to explain errors, concepts and docs; ask for hints | Pasting generated code into the project |
| 2. Review everything | 6 to 12 | Generate code, then read every line, write or extend the tests, and explain it back | Merging code you can't explain or that has no test |
| 3. Pair partner | 13 to 18 | Use it for scaffolding, config, refactors, and as a reviewer of your own code | Letting it decide architecture or incident response for you |

The AI-assisted development track explains why: early on, the struggle is the learning, and a model that writes the code removes it. Each week's "AI this week" box names the current stage and gives one concrete example for that week's work.

## Site architecture

The site is an Astro Starlight project using Starlight's built-in three-column layout, search, dark mode and code blocks, extended with a handful of custom components.

**Layout.** Left sidebar: Start here, Weeks (grouped by the four phases), Tracks (grouped by the five groups). Center: content. Right: Starlight's "On this page" table of contents.

**Diagrams** (revised 2026-10-10 after Casey asked for better theming and walkthroughs):

- Every diagram is D2 text inside a `<Diagram>` MDX component. At build time the `d2` CLI renders it to SVG, which is inlined in the page. Its colors are rewritten to CSS variables mapped to Starlight's theme, so diagrams follow the site's light/dark toggle exactly.
- **Walkthroughs:** a diagram can carry steps, each naming the shapes and arrows to highlight and a caption. Readers click Prev/Next; the named parts light up in the accent color and the rest dims. Without JavaScript the steps show as a numbered list under the diagram. The script is under 2 KB.
- D2 covers flowcharts, architecture, sequence diagrams and tables. Mermaid was dropped: a spike showed D2 gives stable names for every shape and arrow, deterministic output, and needs no headless browser at build time. The home-page timeline is a custom component, not a diagram.

**Custom components** in `src/components/`:

| Component | Purpose |
| --- | --- |
| `WeekHeader` | Time estimate, prerequisites, phase, AI stage, from frontmatter |
| `ObserveIt`, `OperateIt`, `AiThisWeek` | Styled callouts for the three standard boxes |
| `SideLab` | Collapsible optional lab |
| `DoneWhen` | Checklist whose ticks are saved in the reader's browser (`localStorage`) |
| `ReadWith` | Track links for a week, from frontmatter |
| `UsedInWeeks` | Backlinks on a track page, computed from all week frontmatter |
| `WeekTimeline` | Home-page timeline that shows progress from `DoneWhen` state |
| `CodeFrom` | Pulls a snippet from `examples/` by file and region marker |
| `Diagram` | Renders D2 to themed inline SVG, with optional walkthrough steps |

**Week page template:** `WeekHeader`, then Goals, Concepts (with diagrams), Build (steps on the uptime monitor), the three callouts, Side lab, Done when, Read with this week.

**Content schema.** Starlight's content collection schema is extended so week pages require `week` (0 to 18), `phase`, `hours` (range), `aiStage` (1 to 3) and `tracks` (list of track slugs). An unknown track slug or missing field fails the build.

## Repo layout, CI and deploy

One repo holds the site, the tested code examples and one GitHub Actions workflow that checks every PR and publishes `main` to GitHub Pages.

```text
.
├── astro.config.mjs          # Starlight, sidebar, links validator
├── src/
│   ├── content/docs/
│   │   ├── index.mdx         # home: how to use this, timeline
│   │   ├── start-here.mdx
│   │   ├── weeks/00-setup.mdx … 18-sre.mdx
│   │   └── tracks/<group>/<track>.mdx
│   ├── content.config.ts     # frontmatter schema
│   └── components/
├── examples/
│   ├── go/                   # Go module, one package per week step
│   └── web/                  # TS/React snippets, Vitest
├── docs/superpowers/specs/   # this spec
└── .github/workflows/site.yml
```

**Workflow `site.yml`:**

1. Trigger on pull requests and on pushes to `main`.
2. Set up Node and Go; install the `d2` binary, and Playwright Chromium for the end-to-end tests.
3. `go vet` and `go test ./...` in `examples/go`; `npm test` in `examples/web`.
4. `astro build`, with `starlight-links-validator` failing the build on any broken internal link.
5. On `main` only: upload the `dist/` artifact and deploy with `actions/deploy-pages`.

The workflow uses `permissions: pages: write, id-token: write` and a `pages` concurrency group so two deploys never overlap. Pages is set to deploy from GitHub Actions in the repo settings.

## Build plan

The skeleton and one sample week land first in three PRs, then content follows one small PR at a time so each can be reviewed before merge.

**Skeleton (three PRs):**

1. **Site and deploy.** Starlight project, sidebar, the `<Diagram>` component with walkthroughs and one test diagram, the GitHub Actions workflow, Pages deploy, and this spec committed.
2. **Components and stubs.** Frontmatter schema, all custom components, `examples/` scaffolding with its tests in CI, home page with the timeline, and stub pages for all 19 weeks and 25 tracks (frontmatter and headings only).
3. **Sample week: Week 4, Networking.** Fully written as the style reference: sequence diagrams with walkthroughs for DNS resolution and the TLS handshake, a diagram of the request path, Go `httptrace` examples, all three callouts, side lab and Done when. Week 4 is the sample because it exercises every component and diagram type.

**Content (one PR each, after the sample week is approved):**

- Weeks, in order, one week per PR.
- Tracks, two or three related tracks per PR, prioritised by which weeks link to them first.
- Each PR must pass CI (examples tested, links valid) and the built site is attached as a downloadable CI artifact for review (GitHub Pages has no per-PR previews).

## Out of scope and open questions

**Out of scope for now:**

- A complete reference implementation repo of the uptime monitor.
- Accounts, comments, or server-side progress tracking on the site; progress lives only in the reader's browser.
- A custom domain for the site; the default `github.io` address is used.
- Cloud-provider-specific tracks (AWS, GCP); weeks 16 to 18 stay provider-neutral on a small VPS or managed Kubernetes.

**Open questions, to settle during the content PRs:**

- Which queue the uptime monitor uses in week 9 (Postgres-backed queue, NATS, or Redis streams).
- Which cloud box the reader uses in week 16 (a VPS running k3s, or a small managed Kubernetes cluster).

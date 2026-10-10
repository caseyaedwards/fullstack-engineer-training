# Full-Stack Engineer Training

A 19-week training path to becoming a full-stack engineer, built as a static documentation site with Astro and Starlight. Concepts are explained with step-through diagrams (D2, rendered at build time), and internal links are validated on every build. It deploys to GitHub Pages at https://caseyaedwards.github.io/fullstack-engineer-training/.

## Local Development

```sh
npm ci
# Diagrams need the d2 CLI, v0.7.1: https://github.com/terrastruct/d2/releases/tag/v0.7.1
d2 --version

npm run dev          # dev server at http://localhost:4321
npm run build        # production build into dist/ (also validates links)
npm run test:unit    # unit tests
npm run test:build   # tests against the built dist/ (run after build)
npm run test:e2e     # Playwright tests against `astro preview` (run after build)
```

To test under the deployed sub-path, set `BASE_PATH=/fullstack-engineer-training/` for build and tests. Install the browser with `npx playwright install chromium`, or point Playwright at an existing one with `PLAYWRIGHT_CHROMIUM_EXECUTABLE=/path/to/chrome`.

## Writing Style

Site copy follows the [technical writing style guide](docs/style-guide.md): concrete prose with no hedging, no contractions and Title Case headings. `npm run test:build` runs the prose lint in `tests/build/prose.test.mjs`, which enforces the machine-checkable rules.

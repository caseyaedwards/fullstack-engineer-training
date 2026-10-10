// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import rehypeMermaid from 'rehype-mermaid';

const site = process.env.SITE_URL ?? 'http://localhost:4321';
const base = process.env.BASE_PATH ?? '/';

// Optional: point Playwright at a preinstalled Chromium (e.g. sandboxes where
// the bundled Playwright build differs). CI leaves this unset and uses the
// browser from `npx playwright install chromium`.
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE;

export default defineConfig({
  site,
  base,
  markdown: {
    // Keep ```mermaid blocks away from Shiki so rehype-mermaid sees them.
    syntaxHighlight: { type: 'shiki', excludeLangs: ['mermaid'] },
    rehypePlugins: [
      [
        rehypeMermaid,
        {
          strategy: 'img-svg',
          dark: true,
          ...(executablePath ? { launchOptions: { executablePath } } : {}),
        },
      ],
    ],
  },
  integrations: [
    starlight({
      title: 'Full-Stack Engineer Path',
    }),
  ],
});

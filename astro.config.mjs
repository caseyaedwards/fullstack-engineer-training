// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';

const site = process.env.SITE_URL ?? 'http://localhost:4321';
const base = process.env.BASE_PATH ?? '/';

export default defineConfig({
  site,
  base,
  integrations: [
    starlight({
      title: 'Full-Stack Engineer Path',
      plugins: [starlightLinksValidator()],
      customCss: ['./src/styles/diagrams.css'],
    }),
  ],
});

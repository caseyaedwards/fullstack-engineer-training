import { defineConfig } from 'vitest/config';

// Two projects. src/week10/web is the reader's web/ folder at the end of
// Week 10, with its own vite.config.ts (React, jsdom, setup file). Every
// other example runs in Node.
export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: 'examples',
          include: ['src/**/*.test.ts'],
          exclude: ['src/week10/web/**'],
        },
      },
      'src/week10/web',
    ],
  },
});

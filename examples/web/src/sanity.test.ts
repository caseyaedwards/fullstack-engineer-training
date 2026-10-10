import { expect, test } from 'vitest';

// Proves the example tree's test runner works; real examples add their own tests beside their code.
test('vitest runs TypeScript tests', () => {
  const greet = (name: string): string => `hello, ${name}`;
  expect(greet('web')).toBe('hello, web');
});

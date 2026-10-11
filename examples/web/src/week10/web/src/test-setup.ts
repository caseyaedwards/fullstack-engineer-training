// region: setup
// Vitest runs this file before every test file (setupFiles in vite.config.ts).
import '@testing-library/jest-dom/vitest' // adds matchers such as toBeVisible
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// Remove what each test rendered, which keeps one test from finding the
// elements of another.
afterEach(() => {
  cleanup()
})
// endregion: setup

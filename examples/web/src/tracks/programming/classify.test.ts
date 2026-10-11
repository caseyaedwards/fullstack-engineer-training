import { describe, expect, it } from 'vitest'
import { classify, ErrTimeout, StatusError } from './classify'

// region: test
describe('classify', () => {
  it.each([
    ['nil', null, 'ok'],
    ['timeout', ErrTimeout, 'timeout'],
    ['wrapped timeout', new Error('check example.com', { cause: ErrTimeout }), 'timeout'],
    ['wrapped 503', new Error('check example.com', { cause: new StatusError(503) }), 'http-5xx'],
    ['404', new StatusError(404), 'http-4xx'],
    ['other', new Error('connection refused'), 'other'],
  ])('%s', (_name, err, want) => {
    expect(classify(err)).toBe(want)
  })
})
// endregion: test

describe('classify edge cases', () => {
  it('follows three layers of cause', () => {
    const err = new Error('round 7', {
      cause: new Error('worker 2', { cause: new Error('check https://example.com', { cause: ErrTimeout }) }),
    })
    expect(classify(err)).toBe('timeout')
  })

  it('treats a status outside 400 to 599 as other', () => {
    expect(classify(new StatusError(302))).toBe('other')
  })

  it('loses the class when the cause is only copied into the message', () => {
    expect(classify(new Error(`check example.com: ${ErrTimeout.message}`))).toBe('other')
  })

  it('stops on a cause cycle', () => {
    const a = new Error('a')
    const b = new Error('b', { cause: a })
    a.cause = b
    expect(classify(a)).toBe('other')
  })
})

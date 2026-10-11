import { useEffect, useState } from 'react'

// region: hook
// Polled is what a component knows about one request: the last answer,
// the last error, and when the last answer arrived. After a failed
// refresh, data keeps the previous answer and error says why it is old.
export type Polled<T> = {
  data?: T
  error?: Error
  updatedAt?: Date
}

// usePolling calls load once when the component mounts, then again every
// everyMs milliseconds after each answer. When the component unmounts, or
// load changes, it cancels the request in flight and the next timer.
// load must keep its identity between renders: wrap it in useCallback.
export function usePolling<T>(load: (signal: AbortSignal) => Promise<T>, everyMs: number): Polled<T> {
  const [state, setState] = useState<Polled<T>>({})

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined

    async function run() {
      try {
        const data = await load(controller.signal)
        if (controller.signal.aborted) return // the answer came too late
        setState({ data, updatedAt: new Date() })
      } catch (err) {
        if (controller.signal.aborted) return // cancelled on purpose
        setState((prev) => ({ ...prev, error: err as Error }))
      }
      // The next request starts after this one ends, never on top of it.
      timer = setTimeout(run, everyMs)
    }
    run()

    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [load, everyMs])

  return state
}
// endregion: hook

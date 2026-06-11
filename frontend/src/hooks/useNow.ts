import { useEffect, useState } from 'react'

// useNow returns a wall-clock millisecond timestamp that advances on an interval,
// so a component can render a live elapsed timer. It only ticks while `enabled` is
// true (e.g. an incident is in flight), avoiding needless re-renders when idle.
export function useNow(intervalMs = 1000, enabled = true): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!enabled) {
      return
    }
    setNow(Date.now())
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs, enabled])

  return now
}

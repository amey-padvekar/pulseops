import { useEffect, useRef, useState } from 'react'

// useFlashOnChange returns a transient `true` whenever `key` changes value, so a
// component can apply a brief highlight class to draw the eye to a websocket-driven
// update — then settle back to rest. It never flashes on first mount (the initial
// render is not a "change"), keeping the motion meaningful and restrained
// (Phase 12 step 4.8).
export function useFlashOnChange(
  key: string | number | null | undefined,
  durationMs = 1200,
): boolean {
  const [flashing, setFlashing] = useState(false)
  const previousKeyRef = useRef(key)
  const isFirstRef = useRef(true)

  useEffect(() => {
    if (isFirstRef.current) {
      isFirstRef.current = false
      previousKeyRef.current = key
      return
    }

    if (previousKeyRef.current === key) {
      return
    }

    previousKeyRef.current = key
    setFlashing(true)
    const timer = window.setTimeout(() => setFlashing(false), durationMs)
    return () => window.clearTimeout(timer)
  }, [key, durationMs])

  return flashing
}

"use client"

import { useEffect, useState } from "react"

/**
 * Returns a copy of `value` that only updates after `delayMs` of inactivity.
 * Use for search inputs so each keystroke doesn't fire a network request.
 */
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const timeout = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(timeout)
  }, [value, delayMs])

  return debounced
}

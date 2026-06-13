"use client"

import { useCallback, useEffect, useState } from "react"

export interface MatchClock {
  period: number
  elapsedSeconds: number
  running: boolean
  toggle: () => void
  pause: () => void
  endHalf: () => void
  startNextHalf: () => void
}

/**
 * Local, ADVISORY match clock + period. Deliberately client-side only: the
 * score is derived from the append-only event log, never from the clock, so
 * clock drift, pauses, or a reload can never affect the result. Period controls
 * pair with half_started/half_ended events emitted by the caller.
 */
export function useMatchClock(initialPeriod: number): MatchClock {
  const [period, setPeriod] = useState(Math.max(1, initialPeriod))
  const [elapsedSeconds, setElapsed] = useState(0)
  const [running, setRunning] = useState(false)

  useEffect(() => {
    if (!running) return
    const id = setInterval(() => setElapsed((s) => s + 1), 1000)
    return () => clearInterval(id)
  }, [running])

  const toggle = useCallback(() => setRunning((r) => !r), [])
  const pause = useCallback(() => setRunning(false), [])
  const endHalf = useCallback(() => setRunning(false), [])
  const startNextHalf = useCallback(() => {
    setPeriod((p) => p + 1)
    setElapsed(0)
    setRunning(true)
  }, [])

  return { period, elapsedSeconds, running, toggle, pause, endHalf, startNextHalf }
}

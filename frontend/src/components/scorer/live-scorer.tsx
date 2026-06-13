"use client"

import { useCallback, useMemo, useRef, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { SwordsIcon, AlertTriangleIcon, MoreHorizontalIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ScorerHeader } from "./scorer-header"
import { Scoreboard } from "./scoreboard"
import { ReadOnlyTimeline } from "./read-only-timeline"
import { SyncBanner } from "./sync-banner"
import { SyncStatus } from "./sync-status"
import { StartMatchGate } from "./start-match-gate"
import { StartMatchButton } from "./start-match-button"
import { ReadOnlyBanner } from "./read-only-banner"
import { TeamColumn } from "./team-column"
import { UndoButton } from "./undo-button"
import { CompletionGateBar } from "./completion-gate-bar"
import { CompleteMatchDialog } from "./complete-match-dialog"
import { AbandonDialog } from "./abandon-dialog"
import { MoreEventsSheet } from "./more-events-sheet"
import { PeriodControls } from "./period-controls"
import { ConcurrentScorerBanner } from "./concurrent-scorer-banner"
import { useMatch } from "@/hooks/use-matches"
import { useMatchScore } from "@/hooks/use-match-score"
import { useMatchEvents } from "@/hooks/use-match-events"
import { useLocalScore } from "@/hooks/use-local-score"
import { useScoringQueue } from "@/hooks/use-scoring-queue"
import { useMatchClock } from "@/hooks/use-match-clock"
import { useCompleteMatch } from "@/hooks/use-complete-match"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { useAuthStore, selectRole, selectUserId } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { matchEventsApi } from "@/lib/api/match-events"
import { matchKeys } from "@/lib/query-keys"
import { optimisticScore } from "@/lib/scoring/optimistic-score"
import { evaluateCompletion, buildCompletionBody } from "@/lib/scoring/completion-gate"
import { buildGeneric } from "@/lib/scoring/scoring-actions"
import { formatMatchLabel, matchParticipantIds, matchParticipantType } from "@/lib/match-meta"
import type { Match } from "@/types/api/matches"
import type { MatchEvent, CreateMatchEventRequest } from "@/types/api/match-events"

interface LiveScorerProps {
  orgSlug: string
  matchId: string
}

const EMPTY_EVENTS: MatchEvent[] = []
const EVENTS_PARAMS = { effective_only: false, limit: 500, offset: 0 }

/**
 * Live scorer. FE-7BA: read-only shell. FE-7BB: exactly-once scoring queue.
 * FE-7BC: operational completeness — clock/periods, timeouts & player events,
 * gated completion/abandon with winner confirmation, concurrent-scorer
 * awareness, standings refresh, and an accessible spectator/read-only mode.
 */
export function LiveScorer({ orgSlug, matchId }: LiveScorerProps) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const role = useAuthStore(selectRole)
  const currentUserId = useAuthStore(selectUserId)
  const canScore = hasPermission(role, "match.score")
  const canStart = hasPermission(role, "match.update")

  const { data: match, isLoading, isError } = useMatch(orgSlug, matchId)
  const scoreQuery = useMatchScore(orgSlug, matchId, !!match)
  const eventsQuery = useMatchEvents(orgSlug, matchId, !!match)
  const { resolve } = useParticipantNames(orgSlug)

  const serverEvents = useMemo(() => eventsQuery.data?.events ?? EMPTY_EVENTS, [eventsQuery.data])
  const localScore = useLocalScore(match ?? null, match ? serverEvents : null)
  const clock = useMatchClock(1)

  const postEvent = useCallback(
    (body: CreateMatchEventRequest) =>
      matchEventsApi.create(orgSlug, matchId, body).then((r) => r.data),
    [orgSlug, matchId],
  )
  const fetchServerEvents = useCallback(
    () => matchEventsApi.list(orgSlug, matchId, EVENTS_PARAMS).then((r) => r.data.events),
    [orgSlug, matchId],
  )
  const onServerChanged = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: matchKeys.eventsRoot(orgSlug, matchId) })
    queryClient.invalidateQueries({ queryKey: matchKeys.score(orgSlug, matchId) })
  }, [queryClient, orgSlug, matchId])

  const queue = useScoringQueue({ matchId, serverEvents, postEvent, fetchServerEvents, onServerChanged })
  const completeMatch = useCompleteMatch(orgSlug, matchId, match?.tournament_id ?? "")

  function exit() {
    router.push(`/${orgSlug}/matches/${matchId}`)
  }
  function refresh() {
    scoreQuery.refetch()
    eventsQuery.refetch()
  }

  if (isLoading) {
    return (
      <Shell>
        <div className="space-y-4 p-4" aria-busy="true" aria-label="Loading scorer">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      </Shell>
    )
  }

  if (isError || !match) {
    return (
      <Shell>
        <div className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
          <SwordsIcon className="size-10 text-muted-foreground/40" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Match not found</p>
            <p className="text-xs text-muted-foreground">
              This match may have been removed or you may not have access.
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={exit}>
            Back to match
          </Button>
        </div>
      </Shell>
    )
  }

  const isTeam = matchParticipantType(match) === "team"
  const { homeId, awayId } = matchParticipantIds(match)
  const homeName = resolve(isTeam ? homeId : null, isTeam ? null : homeId)
  const awayName = resolve(isTeam ? awayId : null, isTeam ? null : awayId)

  const isLive = match.status === "live"
  const isScheduled = match.status === "scheduled"
  const liveScoringMode = isLive && canScore

  const authoritative = scoreQuery.data
  let homeScore: number | null
  let awayScore: number | null
  let usingLocalFallback = false

  if (liveScoringMode) {
    const opt = optimisticScore(match, serverEvents, queue.actions)
    homeScore = opt.home
    awayScore = opt.away
  } else if (authoritative) {
    homeScore = authoritative.home_score
    awayScore = authoritative.away_score
  } else if (!isLive) {
    homeScore = match.home_score
    awayScore = match.away_score
  } else if (localScore) {
    homeScore = localScore.home
    awayScore = localScore.away
    usingLocalFallback = true
  } else {
    homeScore = null
    awayScore = null
  }

  const eventMaxPeriod = serverEvents.reduce<number | null>((max, e) => {
    if (e.period == null) return max
    return max == null || e.period > max ? e.period : max
  }, null)
  const displayPeriod = liveScoringMode ? clock.period : eventMaxPeriod

  const parityMismatch =
    !liveScoringMode &&
    isLive &&
    !!authoritative &&
    !!localScore &&
    (authoritative.home_score !== localScore.home || authoritative.away_score !== localScore.away)

  const scoreSettled = !!authoritative && !scoreQuery.isFetching
  const completion = evaluateCompletion({
    status: match.status,
    unsyncedCount: queue.unsyncedCount,
    hasFailed: queue.hasFailed,
    score: scoreSettled ? authoritative : undefined,
  })

  // Frontend scorer-ownership awareness: any event recorded by another account.
  const othersPresent = serverEvents.some(
    (e) => e.recorded_by != null && currentUserId != null && e.recorded_by !== currentUserId,
  )

  return (
    <Shell>
      <ScorerHeader
        matchLabel={formatMatchLabel(match)}
        status={match.status}
        period={displayPeriod}
        clock={
          liveScoringMode
            ? { elapsedSeconds: clock.elapsedSeconds, running: clock.running, onToggle: clock.toggle }
            : undefined
        }
        onExit={exit}
      />

      <div className="flex-1 space-y-3 overflow-y-auto p-3 pb-10 sm:space-y-4 sm:p-4">
        <div className="rounded-xl border border-border bg-card px-3 py-5 sm:px-6">
          <Scoreboard
            homeName={homeName}
            awayName={awayName}
            homeScore={homeScore}
            awayScore={awayScore}
            status={match.status}
            isWalkover={match.is_walkover}
          />
        </div>

        {liveScoringMode ? (
          <ScoringMode
            isTeam={isTeam}
            homeName={homeName}
            awayName={awayName}
            homeId={homeId}
            awayId={awayId}
            homeScore={homeScore ?? 0}
            awayScore={awayScore ?? 0}
            clockPeriod={clock.period}
            clockSeconds={clock.elapsedSeconds}
            queue={queue}
            completion={completion}
            completeMatch={completeMatch}
            othersPresent={othersPresent}
            authoritativeHome={authoritative?.home_score ?? null}
            authoritativeAway={authoritative?.away_score ?? null}
            onEndHalf={() => {
              queue.enqueue(
                buildGeneric("half_ended", { period: clock.period, clockSeconds: clock.elapsedSeconds }),
              )
              clock.endHalf()
            }}
            onStartNextHalf={() => {
              queue.enqueue(buildGeneric("half_started", { period: clock.period + 1 }))
              clock.startNextHalf()
            }}
            serverEvents={serverEvents}
            match={match}
            resolveName={resolve}
          />
        ) : (
          <ReadOnlyMode
            match={match}
            isScheduled={isScheduled}
            canStart={canStart}
            orgSlug={orgSlug}
            matchId={matchId}
            scoreQuery={scoreQuery}
            eventsQuery={eventsQuery}
            serverEvents={serverEvents}
            resolveName={resolve}
            usingLocalFallback={usingLocalFallback}
            parityMismatch={parityMismatch}
            headlineHome={homeScore}
            headlineAway={awayScore}
            othersPresent={othersPresent && isLive}
            onRefresh={refresh}
          />
        )}
      </div>
    </Shell>
  )
}

// ── Scoring mode ────────────────────────────────────────────────────────────

function ScoringMode({
  isTeam,
  homeName,
  awayName,
  homeId,
  awayId,
  homeScore,
  awayScore,
  clockPeriod,
  clockSeconds,
  queue,
  completion,
  completeMatch,
  othersPresent,
  authoritativeHome,
  authoritativeAway,
  onEndHalf,
  onStartNextHalf,
  serverEvents,
  match,
  resolveName,
}: {
  isTeam: boolean
  homeName: string
  awayName: string
  homeId: string | null
  awayId: string | null
  homeScore: number
  awayScore: number
  clockPeriod: number
  clockSeconds: number
  queue: ReturnType<typeof useScoringQueue>
  completion: ReturnType<typeof evaluateCompletion>
  completeMatch: ReturnType<typeof useCompleteMatch>
  othersPresent: boolean
  authoritativeHome: number | null
  authoritativeAway: number | null
  onEndHalf: () => void
  onStartNextHalf: () => void
  serverEvents: MatchEvent[]
  match: Match
  resolveName: (teamId: string | null, playerId: string | null) => string
}) {
  const [completeOpen, setCompleteOpen] = useState(false)
  const [abandonOpen, setAbandonOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)
  // Guards a terminal transition from firing twice before isPending re-renders
  // (defence-in-depth; the backend also rejects a second terminal PATCH).
  const submittingRef = useRef(false)

  const homeAttr = { teamMode: isTeam, participantId: homeId ?? "" }
  const awayAttr = { teamMode: isTeam, participantId: awayId ?? "" }
  const disabled = !homeId || !awayId
  const canAbandon = queue.unsyncedCount === 0 && !queue.hasFailed

  function confirmComplete() {
    if (!completion.winner || submittingRef.current) return
    submittingRef.current = true
    completeMatch.mutate(buildCompletionBody(isTeam, completion.winner), {
      onSuccess: () => {
        setCompleteOpen(false)
        toast.success("Match completed")
      },
      onSettled: () => {
        submittingRef.current = false
      },
    })
  }
  function confirmAbandon() {
    if (submittingRef.current) return
    submittingRef.current = true
    completeMatch.mutate(
      { status: "abandoned" },
      {
        onSuccess: () => {
          setAbandonOpen(false)
          toast.success("Match abandoned")
        },
        onSettled: () => {
          submittingRef.current = false
        },
      },
    )
  }

  return (
    <>
      {/* Accessible live announcement of the current score. */}
      <p className="sr-only" role="status" aria-live="polite">
        {homeName} {homeScore}, {awayName} {awayScore}
      </p>

      {othersPresent && <ConcurrentScorerBanner />}

      <SyncStatus
        isOnline={queue.isOnline}
        isSyncing={queue.isSyncing}
        unsyncedCount={queue.unsyncedCount}
        hasFailed={queue.hasFailed}
      />

      <div className="grid grid-cols-2 gap-2 sm:gap-3">
        <TeamColumn
          name={homeName}
          opponentName={awayName}
          attribution={homeAttr}
          period={clockPeriod}
          disabled={disabled}
          onAction={queue.enqueue}
          align="right"
        />
        <TeamColumn
          name={awayName}
          opponentName={homeName}
          attribution={awayAttr}
          period={clockPeriod}
          disabled={disabled}
          onAction={queue.enqueue}
          align="left"
        />
      </div>

      <div className="grid grid-cols-2 gap-2">
        <UndoButton target={queue.undoTarget} onUndo={queue.undo} />
        <button
          type="button"
          onClick={() => setMoreOpen(true)}
          className="flex min-h-12 items-center justify-center gap-2 rounded-xl border-2 border-border bg-card px-4 text-sm font-semibold transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <MoreHorizontalIcon className="size-4" />
          More
        </button>
      </div>

      <PeriodControls
        period={clockPeriod}
        disabled={false}
        onEndHalf={onEndHalf}
        onStartNextHalf={onStartNextHalf}
      />

      <CompletionGateBar
        readiness={completion}
        canAbandon={canAbandon}
        onComplete={() => setCompleteOpen(true)}
        onAbandon={() => setAbandonOpen(true)}
      />

      <section aria-label="Event history" className="space-y-2 pt-2">
        <h2 className="px-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Event history
        </h2>
        <ReadOnlyTimeline events={serverEvents} match={match} resolveName={resolveName} />
      </section>

      <MoreEventsSheet
        open={moreOpen}
        onOpenChange={setMoreOpen}
        isTeam={isTeam}
        homeName={homeName}
        awayName={awayName}
        homeId={homeId}
        awayId={awayId}
        period={clockPeriod}
        clockSeconds={clockSeconds}
        onEnqueue={queue.enqueue}
      />
      <CompleteMatchDialog
        open={completeOpen}
        onOpenChange={setCompleteOpen}
        homeName={homeName}
        awayName={awayName}
        homeScore={authoritativeHome ?? homeScore}
        awayScore={authoritativeAway ?? awayScore}
        winner={completion.winner}
        isPending={completeMatch.isPending}
        onConfirm={confirmComplete}
      />
      <AbandonDialog
        open={abandonOpen}
        onOpenChange={setAbandonOpen}
        isPending={completeMatch.isPending}
        onConfirm={confirmAbandon}
      />
    </>
  )
}

// ── Read-only / spectator mode (FE-7BA behaviour, preserved + polished) ──────

function ReadOnlyMode({
  match,
  isScheduled,
  canStart,
  orgSlug,
  matchId,
  scoreQuery,
  eventsQuery,
  serverEvents,
  resolveName,
  usingLocalFallback,
  parityMismatch,
  headlineHome,
  headlineAway,
  othersPresent,
  onRefresh,
}: {
  match: Match
  isScheduled: boolean
  canStart: boolean
  orgSlug: string
  matchId: string
  scoreQuery: ReturnType<typeof useMatchScore>
  eventsQuery: ReturnType<typeof useMatchEvents>
  serverEvents: MatchEvent[]
  resolveName: (teamId: string | null, playerId: string | null) => string
  usingLocalFallback: boolean
  parityMismatch: boolean
  headlineHome: number | null
  headlineAway: number | null
  othersPresent: boolean
  onRefresh: () => void
}) {
  return (
    <>
      <ReadOnlyBanner status={match.status} />

      {othersPresent && <ConcurrentScorerBanner />}

      {!isScheduled && (
        <SyncBanner
          updatedAt={scoreQuery.dataUpdatedAt || null}
          isFetching={scoreQuery.isFetching || eventsQuery.isFetching}
          isError={scoreQuery.isError}
          onRefresh={onRefresh}
        />
      )}

      {usingLocalFallback && (
        <p className="px-1 text-xs text-muted-foreground" role="status">
          Showing the score computed locally from the event log while the server score syncs.
        </p>
      )}

      {parityMismatch && (
        <div
          role="status"
          className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200"
        >
          <AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
          <span>
            Reconciling the displayed score with the event log. The server score (
            {headlineHome}–{headlineAway}) is authoritative.
          </span>
        </div>
      )}

      {isScheduled ? (
        canStart ? (
          <div className="space-y-3">
            <StartMatchButton orgSlug={orgSlug} matchId={matchId} />
            <p className="text-center text-xs text-muted-foreground">
              Starting the match opens live scoring.
            </p>
          </div>
        ) : (
          <StartMatchGate scheduledAt={match.scheduled_at} />
        )
      ) : (
        <section aria-label="Event history" className="space-y-2">
          <h2 className="px-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Event history
          </h2>
          {eventsQuery.isLoading ? (
            <div className="space-y-1.5" aria-busy="true" aria-label="Loading events">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-11 w-full" />
              ))}
            </div>
          ) : eventsQuery.isError ? (
            <div className="flex flex-col items-start gap-2 rounded-lg border border-border px-3 py-4">
              <p className="text-sm text-muted-foreground">Failed to load the event history.</p>
              <Button variant="outline" size="sm" onClick={() => eventsQuery.refetch()}>
                Retry
              </Button>
            </div>
          ) : (
            <ReadOnlyTimeline events={serverEvents} match={match} resolveName={resolveName} />
          )}
        </section>
      )}
    </>
  )
}

// Full-bleed fixed overlay (covers the org shell without modifying FE-7A).
function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-background text-foreground">{children}</div>
  )
}

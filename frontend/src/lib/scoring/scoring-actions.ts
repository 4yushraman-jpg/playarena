import type { CreateMatchEventRequest, MatchEventType } from "@/types/api/match-events"
import { newClientEventId } from "./client-event-id"

/**
 * Pure mapping from a scorer's intent to a CreateMatchEventRequest. Every action
 * carries a client_event_id in its payload (the exactly-once key) and routes the
 * participant to the team_id or player_id slot per match format. This is the
 * single place UI intent becomes an event body — so the slot/payload mapping is
 * unit-testable in isolation.
 */

export interface AttributedTo {
  teamMode: boolean
  participantId: string
}

export interface ActionContext {
  period?: number | null
}

export interface BuiltAction {
  clientEventId: string
  body: CreateMatchEventRequest
  /** True for point-bearing actions (eligible for undo via correction). */
  isScoring: boolean
  /** For corrections: the server id of the event being cancelled. */
  cancelsServerId?: string
}

function participantField(a: AttributedTo): { team_id?: string; player_id?: string } {
  return a.teamMode ? { team_id: a.participantId } : { player_id: a.participantId }
}

function periodField(ctx?: ActionContext): { period?: number } {
  const p = ctx?.period
  return p != null ? { period: p } : {}
}

function build(
  eventType: MatchEventType,
  a: AttributedTo,
  payload: Record<string, unknown>,
  ctx: ActionContext | undefined,
  isScoring: boolean,
): BuiltAction {
  const clientEventId = newClientEventId()
  return {
    clientEventId,
    isScoring,
    body: {
      event_type: eventType,
      ...participantField(a),
      ...periodField(ctx),
      payload: { ...payload, client_event_id: clientEventId },
    },
  }
}

/** Successful raid — payload.points (>0) to the raiding side. */
export function buildRaid(a: AttributedTo, points: number, ctx?: ActionContext): BuiltAction {
  return build("raid_successful", a, { points }, ctx, true)
}

/** Bonus point — +1 to the raiding side. */
export function buildBonus(a: AttributedTo, ctx?: ActionContext): BuiltAction {
  return build("bonus_point_awarded", a, {}, ctx, true)
}

/** Successful tackle — +1 to the defending side. */
export function buildTackle(a: AttributedTo, ctx?: ActionContext): BuiltAction {
  return build("tackle_successful", a, {}, ctx, true)
}

/** Super tackle — +2 to the defending side. */
export function buildSuperTackle(a: AttributedTo, ctx?: ActionContext): BuiltAction {
  return build("super_tackle", a, {}, ctx, true)
}

/** Penalty — payload.points (>0) to the attributed side. */
export function buildPenalty(a: AttributedTo, points: number, ctx?: ActionContext): BuiltAction {
  return build("penalty_awarded", a, { points }, ctx, true)
}

/**
 * All out — the ELIMINATED side is identified by `eliminated`; the backend
 * awards bonus_points to the opponent. The event is attributed to the eliminated
 * side and the payload carries the eliminated team_id (the engine keys on it).
 */
export function buildAllOut(
  eliminated: AttributedTo,
  bonusPoints: number,
  ctx?: ActionContext,
): BuiltAction {
  // all_out is keyed on payload.team_id; only meaningful in team-format matches.
  return build(
    "all_out",
    eliminated,
    { team_id: eliminated.participantId, bonus_points: bonusPoints },
    ctx,
    true,
  )
}

/**
 * Generic non-scoring event (lifecycle, timeout, player-state, administrative).
 * Carries a client_event_id for exactly-once delivery; contributes 0 to the
 * score. `attribution` is optional (e.g. a match-level timeout has no team).
 */
export function buildGeneric(
  eventType: MatchEventType,
  opts: {
    attribution?: AttributedTo
    payload?: Record<string, unknown>
    period?: number | null
    clockSeconds?: number | null
  } = {},
): BuiltAction {
  const clientEventId = newClientEventId()
  const part = opts.attribution ? participantField(opts.attribution) : {}
  const period = opts.period != null ? { period: opts.period } : {}
  const clock = opts.clockSeconds != null ? { clock_seconds: opts.clockSeconds } : {}
  return {
    clientEventId,
    isScoring: false,
    body: {
      event_type: eventType,
      ...part,
      ...period,
      ...clock,
      payload: { ...(opts.payload ?? {}), client_event_id: clientEventId },
    },
  }
}

/**
 * Score correction — cancels a previously-confirmed event by its SERVER id.
 * Carries its own client_event_id for exactly-once delivery of the correction
 * itself. Not a scoring action (contributes 0); it negates its target.
 */
export function buildCorrection(targetServerId: string, ctx?: ActionContext): BuiltAction {
  const clientEventId = newClientEventId()
  return {
    clientEventId,
    isScoring: false,
    cancelsServerId: targetServerId,
    body: {
      event_type: "score_correction",
      cancels_event_id: targetServerId,
      ...periodField(ctx),
      payload: { client_event_id: clientEventId },
    },
  }
}

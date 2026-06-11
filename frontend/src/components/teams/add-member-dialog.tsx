"use client"

import * as React from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Loader2Icon, SearchIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { FormField, FormSelect } from "@/components/ui/form-field"
import { AvatarDisplay } from "@/components/players/player-avatar"
import { usePlayerList } from "@/hooks/use-players"
import { useAddTeamMember } from "@/hooks/use-team-members"

// ── Schema ─────────────────────────────────────────────────────────────────────

const ROLE_OPTIONS = [
  { value: "player", label: "Player" },
  { value: "captain", label: "Captain" },
  { value: "vice_captain", label: "Vice Captain" },
]

const schema = z.object({
  player_id: z.string().min(1, "Select a player"),
  role: z.string().optional(),
  jersey_number: z.string().max(10).optional().or(z.literal("")),
  notes: z.string().max(200).optional().or(z.literal("")),
})

type FormValues = z.infer<typeof schema>

// ── Component ─────────────────────────────────────────────────────────────────

interface AddMemberDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  teamId: string
  /** player_id UUIDs of players already on this team — used to prevent duplicate adds */
  existingMemberPlayerIds?: string[]
}

export function AddMemberDialog({
  open,
  onOpenChange,
  orgSlug,
  teamId,
  existingMemberPlayerIds = [],
}: AddMemberDialogProps) {
  const [search, setSearch] = React.useState("")
  const debouncedSearch = useDebounce(search, 300)

  const { data, isLoading } = usePlayerList(orgSlug, {
    search: debouncedSearch || undefined,
    status: "active",
    limit: 20,
  })

  const addMember = useAddTeamMember(orgSlug, teamId)

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { player_id: "", role: "player", jersey_number: "", notes: "" },
  })

  // eslint-disable-next-line react-hooks/incompatible-library -- RHF watch; known React Compiler incompatibility, non-actionable
  const selectedPlayerId = form.watch("player_id")
  const selectedPlayer = data?.players.find((p) => p.id === selectedPlayerId)

  function handleSubmit(values: FormValues) {
    addMember.mutate(
      {
        player_id: values.player_id,
        role: values.role || undefined,
        jersey_number: values.jersey_number || undefined,
        notes: values.notes || undefined,
      },
      {
        onSuccess: () => {
          form.reset()
          setSearch("")
          onOpenChange(false)
        },
      },
    )
  }

  function handleOpenChange(v: boolean) {
    if (!v) {
      form.reset()
      setSearch("")
    }
    onOpenChange(v)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Add member</DialogTitle>
          <DialogDescription>
            Search for an active player and assign them to this team.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} noValidate className="space-y-4">
          {/* Player search */}
          <div className="space-y-2">
            <div className="relative">
              <SearchIcon className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search players…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-8"
                aria-label="Search players"
              />
            </div>

            {/* Player list */}
            <div
              role="listbox"
              aria-label="Players"
              className="max-h-40 overflow-y-auto rounded-lg border border-border"
            >
              {isLoading ? (
                <div className="flex items-center justify-center py-6">
                  <Loader2Icon className="size-4 animate-spin text-muted-foreground" />
                </div>
              ) : !data?.players.length ? (
                <div className="py-6 text-center text-sm text-muted-foreground">
                  No active players found
                </div>
              ) : (
                data.players.map((player) => {
                  const alreadyOnTeam = existingMemberPlayerIds.includes(player.id)
                  return (
                    <button
                      key={player.id}
                      type="button"
                      role="option"
                      aria-selected={selectedPlayerId === player.id}
                      aria-disabled={alreadyOnTeam}
                      disabled={alreadyOnTeam}
                      onClick={() => {
                        if (!alreadyOnTeam) {
                          form.setValue("player_id", player.id, { shouldValidate: true })
                        }
                      }}
                      className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors
                        ${alreadyOnTeam ? "cursor-not-allowed opacity-50" : "hover:bg-muted/60"}
                        ${selectedPlayerId === player.id ? "bg-primary/10" : ""}`}
                    >
                      <AvatarDisplay
                        displayName={player.display_name}
                        size="sm"
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium">{player.display_name}</p>
                        {player.position && (
                          <p className="text-xs text-muted-foreground">{player.position}</p>
                        )}
                      </div>
                      <div className="flex shrink-0 items-center gap-1.5">
                        {player.jersey_number && (
                          <span className="text-xs text-muted-foreground">#{player.jersey_number}</span>
                        )}
                        {alreadyOnTeam && (
                          <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                            On team
                          </span>
                        )}
                      </div>
                    </button>
                  )
                })
              )}
            </div>

            {form.formState.errors.player_id && (
              <p className="text-xs text-destructive" role="alert">
                {form.formState.errors.player_id.message}
              </p>
            )}
          </div>

          {/* Selected player preview */}
          {selectedPlayer && (
            <div className="flex items-center gap-3 rounded-lg bg-muted/40 px-3 py-2.5">
              <AvatarDisplay displayName={selectedPlayer.display_name} size="sm" />
              <p className="text-sm font-medium">{selectedPlayer.display_name}</p>
            </div>
          )}

          {/* Role + jersey */}
          <div className="grid gap-4 sm:grid-cols-2">
            <FormSelect
              control={form.control}
              name="role"
              label="Role"
              options={ROLE_OPTIONS}
              placeholder="Select role…"
            />
            <FormField
              control={form.control}
              name="jersey_number"
              label="Jersey number"
              placeholder="Optional"
            />
          </div>

          <DialogFooter className="gap-2 sm:gap-0">
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={addMember.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={addMember.isPending || !selectedPlayerId} className="gap-2">
              {addMember.isPending && <Loader2Icon className="size-3.5 animate-spin" />}
              Add member
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ── Debounce ──────────────────────────────────────────────────────────────────

function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = React.useState(value)
  React.useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(id)
  }, [value, delay])
  return debounced
}

package teams

import "errors"

var (
	ErrTeamNotFound         = errors.New("team not found")
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrPlayerNotFound       = errors.New("player not found")
	ErrMembershipNotFound   = errors.New("membership not found")

	// ErrForbidden is returned when the caller's JWT org context does not match
	// the target organization (BOLA / IDOR prevention).
	ErrForbidden = errors.New("access denied: you do not have permission to modify this organization's teams")

	ErrSlugAlreadyTaken     = errors.New("a team with this slug already exists in the organization")
	ErrSlugGenerationFailed = errors.New("could not generate a unique slug for this team name — try providing a more distinctive name")

	ErrInvalidStatus    = errors.New("invalid team status; valid values: active, inactive, disbanded")
	ErrInvalidColor     = errors.New("color must be a 6-digit hex code (e.g. #FF5733)")
	ErrInvalidShortName = errors.New("short_name must be 2–10 characters")

	// ErrMembershipAlreadyActive is returned when a player already holds an
	// active membership on the target team. Historical rows are retained; a new
	// membership row is only created when no active one exists.
	ErrMembershipAlreadyActive = errors.New("player already has an active membership on this team")

	// ErrCrossOrgMembership is returned when the player or team does not belong
	// to the organization identified by the URL slug. Validated in the service
	// layer before the DB trigger trg_team_memberships_org_consistency fires.
	ErrCrossOrgMembership = errors.New("player and team must both belong to the same organization")

	ErrInvalidMembershipRole = errors.New("invalid membership role; valid values: player, captain, vice_captain, coach, manager, support_staff")
)

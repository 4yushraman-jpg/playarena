package players

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// SelfService implements the GP-1 global PlayerProfile use-cases. Unlike the
// org-scoped Service, ownership here is the user themselves: a profile is owned
// by the user whose user_id matches it. There is no org context.
type SelfService struct {
	repo *Repository
	log  *slog.Logger
}

// NewSelfService constructs a SelfService.
func NewSelfService(repo *Repository, log *slog.Logger) *SelfService {
	return &SelfService{repo: repo, log: log}
}

// ── request DTOs ───────────────────────────────────────────────────────────────

// CreateProfileRequest is the payload for POST /api/v1/me/player.
type CreateProfileRequest struct {
	DisplayName  string  `json:"display_name"  validate:"required,min=1,max=255"`
	JerseyNumber *string `json:"jersey_number"`
	Position     *string `json:"position"`
	HeightCm     *int16  `json:"height_cm"`
	WeightKg     *int16  `json:"weight_kg"`
	DominantHand *string `json:"dominant_hand"`
	Nationality  *string `json:"nationality"`
	DateOfBirth  *string `json:"date_of_birth"`
	Bio          *string `json:"bio"`
	Visibility   *string `json:"visibility"` // public | unlisted | private (default private)
}

// UpdateProfileRequest is the payload for PATCH /api/v1/me/player. All fields
// optional. organization_id, user_id and status are intentionally absent: they
// are not user-editable identity fields.
type UpdateProfileRequest struct {
	DisplayName  *string `json:"display_name" validate:"omitempty,min=1,max=255"`
	JerseyNumber *string `json:"jersey_number"`
	Position     *string `json:"position"`
	HeightCm     *int16  `json:"height_cm"`
	WeightKg     *int16  `json:"weight_kg"`
	DominantHand *string `json:"dominant_hand"`
	Nationality  *string `json:"nationality"`
	DateOfBirth  *string `json:"date_of_birth"`
	Bio          *string `json:"bio"`
	Visibility   *string `json:"visibility"`
}

// ── use-cases ──────────────────────────────────────────────────────────────────

// CreateOwn creates the caller's global PlayerProfile (one per user). Returns
// ErrProfileExists when the caller already has one.
func (s *SelfService) CreateOwn(ctx context.Context, actorID string, req CreateProfileRequest) (*Response, error) {
	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	// Fast pre-check; the partial unique index is the authoritative backstop
	// against the create race (mapped to ErrProfileExists in the repository).
	if existing, err := s.repo.GetProfileByUserID(ctx, actorUID); err == nil && existing != nil {
		return nil, ErrProfileExists
	} else if err != nil && !errors.Is(err, ErrPlayerNotFound) {
		return nil, err
	}

	if err := validateDominantHand(req.DominantHand); err != nil {
		return nil, err
	}
	nationality, err := normalizeNationality(req.Nationality)
	if err != nil {
		return nil, err
	}
	dob, err := parseDateOfBirth(req.DateOfBirth)
	if err != nil {
		return nil, err
	}
	visibility, err := normalizeVisibility(req.Visibility, "private")
	if err != nil {
		return nil, err
	}

	profile, err := s.repo.CreateGlobalProfile(ctx, db.CreateGlobalPlayerProfileParams{
		UserID:       actorUID,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		JerseyNumber: req.JerseyNumber,
		Position:     req.Position,
		HeightCm:     req.HeightCm,
		WeightKg:     req.WeightKg,
		DominantHand: req.DominantHand,
		Nationality:  nationality,
		DateOfBirth:  dob,
		Bio:          req.Bio,
		Visibility:   visibility,
	})
	if err != nil {
		return nil, err
	}
	return playerToResponse(profile), nil
}

// GetOwn returns the caller's profile, or ErrPlayerNotFound when none exists.
func (s *SelfService) GetOwn(ctx context.Context, actorID string) (*Response, error) {
	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}
	profile, err := s.repo.GetProfileByUserID(ctx, actorUID)
	if err != nil {
		return nil, err
	}
	return playerToResponse(profile), nil
}

// UpdateOwn applies a partial identity update to the caller's profile.
func (s *SelfService) UpdateOwn(ctx context.Context, actorID string, req UpdateProfileRequest) (*Response, error) {
	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	current, err := s.repo.GetProfileByUserID(ctx, actorUID)
	if err != nil {
		return nil, err
	}

	params := db.UpdateOwnPlayerProfileParams{
		ID:           current.ID,
		UserID:       actorUID,
		DisplayName:  current.DisplayName,
		JerseyNumber: current.JerseyNumber,
		Position:     current.Position,
		HeightCm:     current.HeightCm,
		WeightKg:     current.WeightKg,
		DominantHand: current.DominantHand,
		Nationality:  current.Nationality,
		DateOfBirth:  current.DateOfBirth,
		Bio:          current.Bio,
		Visibility:   current.Visibility,
	}

	if req.DisplayName != nil {
		params.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.JerseyNumber != nil {
		params.JerseyNumber = req.JerseyNumber
	}
	if req.Position != nil {
		params.Position = req.Position
	}
	if req.HeightCm != nil {
		params.HeightCm = req.HeightCm
	}
	if req.WeightKg != nil {
		params.WeightKg = req.WeightKg
	}
	if req.DominantHand != nil {
		if err := validateDominantHand(req.DominantHand); err != nil {
			return nil, err
		}
		params.DominantHand = req.DominantHand
	}
	if req.Nationality != nil {
		nat, err := normalizeNationality(req.Nationality)
		if err != nil {
			return nil, err
		}
		params.Nationality = nat
	}
	if req.DateOfBirth != nil {
		dob, err := parseDateOfBirth(req.DateOfBirth)
		if err != nil {
			return nil, err
		}
		params.DateOfBirth = dob
	}
	if req.Bio != nil {
		params.Bio = req.Bio
	}
	if req.Visibility != nil {
		vis, err := normalizeVisibility(req.Visibility, current.Visibility)
		if err != nil {
			return nil, err
		}
		params.Visibility = vis
	}

	updated, err := s.repo.UpdateOwnProfile(ctx, params)
	if err != nil {
		return nil, err
	}
	return playerToResponse(updated), nil
}

// GetByIDGlobal returns a profile by id, honoring visibility:
//   - the owner and platform admins always see it;
//   - public and unlisted profiles are readable by any authenticated user;
//   - private profiles return ErrPlayerNotFound to non-owners (existence hidden).
func (s *SelfService) GetByIDGlobal(ctx context.Context, id, actorID string, isPlatform bool) (*Response, error) {
	pid, err := pgutil.ParseUUID(id)
	if err != nil {
		return nil, ErrPlayerNotFound
	}
	profile, err := s.repo.GetProfileByID(ctx, pid)
	if err != nil {
		return nil, err
	}

	isOwner := profile.UserID.Valid && pgutil.UUIDToString(profile.UserID) == actorID
	if isOwner || isPlatform {
		return playerToResponse(profile), nil
	}
	switch profile.Visibility {
	case "public", "unlisted":
		return playerToResponse(profile), nil
	default: // private
		return nil, ErrPlayerNotFound
	}
}

// ── helpers ────────────────────────────────────────────────────────────────────

// normalizeVisibility validates an optional visibility value, returning the
// fallback when nil/empty.
func normalizeVisibility(v *string, fallback string) (string, error) {
	if v == nil || *v == "" {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(*v)) {
	case "public":
		return "public", nil
	case "unlisted":
		return "unlisted", nil
	case "private":
		return "private", nil
	}
	return "", ErrInvalidVisibility
}

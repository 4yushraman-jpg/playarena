package webhooks

import (
	"context"
	"encoding/base64"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements webhook endpoint use-cases.
type Service struct {
	repo      *Repository
	secretKey []byte // 32-byte AES-256-GCM key
	log       *slog.Logger
}

// NewService constructs a Service. secretKeyB64 must be a base64-encoded 32-byte key.
func NewService(repo *Repository, secretKeyB64 string, log *slog.Logger) (*Service, error) {
	key, err := base64.StdEncoding.DecodeString(secretKeyB64)
	if err != nil {
		// Try RawStdEncoding (no padding) as a fallback.
		key, err = base64.RawStdEncoding.DecodeString(secretKeyB64)
		if err != nil {
			return nil, err
		}
	}
	if len(key) != 32 {
		return nil, ErrInvalidURL // reuse generic error; caller surfaces config issue
	}
	return &Service{repo: repo, secretKey: key, log: log}, nil
}

// Create validates and registers a new webhook endpoint.
// Returns a CreateResponse with the raw secret (shown once only).
func (s *Service) Create(ctx context.Context, orgID pgtype.UUID, actorID string, req CreateRequest) (*CreateResponse, error) {
	if err := ValidateURL(req.URL); err != nil {
		return nil, err
	}

	rawSecret, err := GenerateSecret()
	if err != nil {
		return nil, err
	}

	ciphertext, err := EncryptSecret(s.secretKey, rawSecret)
	if err != nil {
		return nil, err
	}

	creatorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, err
	}

	ep, err := s.repo.Create(ctx, db.CreateWebhookEndpointParams{
		OrganizationID:   orgID,
		Url:              req.URL,
		SecretCiphertext: ciphertext,
		Description:      req.Description,
		Active:           true,
		CreatedBy:        creatorUID,
	})
	if err != nil {
		return nil, err
	}

	return &CreateResponse{
		Response:  toResponse(ep),
		RawSecret: rawSecret,
	}, nil
}

// GetByID fetches a webhook endpoint, asserting org ownership.
func (s *Service) GetByID(ctx context.Context, orgID pgtype.UUID, webhookID string) (*Response, error) {
	id, err := pgutil.ParseUUID(webhookID)
	if err != nil {
		return nil, ErrWebhookNotFound
	}
	ep, err := s.repo.GetByID(ctx, id, orgID)
	if err != nil {
		return nil, err
	}
	r := toResponse(ep)
	return &r, nil
}

// List returns all webhook endpoints for the organization.
func (s *Service) List(ctx context.Context, orgID pgtype.UUID) (*ListResponse, error) {
	eps, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	resp := make([]Response, 0, len(eps))
	for i := range eps {
		resp = append(resp, toResponse(&eps[i]))
	}
	return &ListResponse{Endpoints: resp, Total: len(resp)}, nil
}

// UpdateActive toggles the active flag on a webhook endpoint.
func (s *Service) UpdateActive(ctx context.Context, orgID pgtype.UUID, webhookID string, active bool) (*Response, error) {
	id, err := pgutil.ParseUUID(webhookID)
	if err != nil {
		return nil, ErrWebhookNotFound
	}
	ep, err := s.repo.UpdateActive(ctx, id, orgID, active)
	if err != nil {
		return nil, err
	}
	r := toResponse(ep)
	return &r, nil
}

// Delete removes a webhook endpoint.
func (s *Service) Delete(ctx context.Context, orgID pgtype.UUID, webhookID string) error {
	id, err := pgutil.ParseUUID(webhookID)
	if err != nil {
		return ErrWebhookNotFound
	}
	return s.repo.Delete(ctx, id, orgID)
}

// toResponse converts a db.WebhookEndpoint to the API response type.
// secret_ciphertext is never included.
func toResponse(ep *db.WebhookEndpoint) Response {
	return Response{
		ID:             pgutil.UUIDToString(ep.ID),
		OrganizationID: pgutil.UUIDToString(ep.OrganizationID),
		URL:            ep.Url,
		Description:    ep.Description,
		Active:         ep.Active,
		CreatedAt:      ep.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:      ep.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

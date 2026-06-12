package media

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/media/storage"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all media endpoints under /api/v1/organizations/{slug}/media.
//
// Permission requirements:
//
//	POST   /          media.upload  (upload a new attachment)
//	GET    /          RequireAuth   (list attachments for an org or entity)
//	GET    /{id}      RequireAuth   (fetch a single attachment)
//	PATCH  /{id}      media.update  (update alt_text, sort_order, is_primary)
//	DELETE /{id}      media.delete  (delete attachment + storage objects)
//
// In development (local backend) a static file server is also mounted at
// /media/files/* so uploaded images can be served without a CDN.
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
	authz *auth.AuthorizationService,
	backend storage.Backend,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, backend, log)
	h := NewHandler(svc, log)

	r.Route("/api/v1/organizations/{slug}/media", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		// Upload — requires media.upload
		r.With(auth.RequirePermission(authz, "media.upload")).
			Post("/", h.Upload)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{id}", h.GetByID)

		// Update — requires media.update
		r.With(auth.RequirePermission(authz, "media.update")).
			Patch("/{id}", h.Update)

		// Delete — requires media.delete
		r.With(auth.RequirePermission(authz, "media.delete")).
			Delete("/{id}", h.Delete)
	})

	// Development-only static file server.
	// In production the CDN (S3 + CloudFront/R2) serves files directly.
	if cfg.StorageBackend == "" || cfg.StorageBackend == "local" {
		localPath := cfg.StorageLocalPath
		if localPath == "" {
			localPath = "./uploads"
		}
		// Ensure the directory exists so the file server doesn't panic.
		_ = os.MkdirAll(localPath, 0o755)
		fs := http.StripPrefix("/media/files/", http.FileServer(http.Dir(localPath)))
		r.Get("/media/files/*", func(w http.ResponseWriter, req *http.Request) {
			fs.ServeHTTP(w, req)
		})
	}
}

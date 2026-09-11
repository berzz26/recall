package local_source

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const fields = `id, name, path, enabled, created_at, updated_at`

func scan(row interface{ Scan(dest ...any) error }) (*LocalSource, error) {
	var s LocalSource
	err := row.Scan(&s.ID, &s.Name, &s.Path, &s.Enabled, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) Create(ctx context.Context, name, path string) (*LocalSource, error) {
	query := fmt.Sprintf(`INSERT INTO local_sources (name, path) VALUES ($1, $2) RETURNING %s`, fields)
	row := r.db.QueryRow(ctx, query, name, path)
	return scan(row)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*LocalSource, error) {
	query := fmt.Sprintf(`SELECT %s FROM local_sources WHERE id = $1`, fields)
	row := r.db.QueryRow(ctx, query, id)
	return scan(row)
}

func (r *Repository) GetByPath(ctx context.Context, path string) (*LocalSource, error) {
	query := fmt.Sprintf(`SELECT %s FROM local_sources WHERE path = $1`, fields)
	row := r.db.QueryRow(ctx, query, path)
	return scan(row)
}

func (r *Repository) List(ctx context.Context) ([]LocalSource, error) {
	query := fmt.Sprintf(`SELECT %s FROM local_sources ORDER BY created_at DESC`, fields)
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []LocalSource
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	if list == nil {
		list = []LocalSource{}
	}
	return list, nil
}

func (r *Repository) ListEnabled(ctx context.Context) ([]LocalSource, error) {
	query := fmt.Sprintf(`SELECT %s FROM local_sources WHERE enabled = true ORDER BY created_at DESC`, fields)
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []LocalSource
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	if list == nil {
		list = []LocalSource{}
	}
	return list, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM local_sources WHERE id = $1`, id)
	return err
}

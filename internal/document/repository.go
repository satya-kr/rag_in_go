package document

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) Create(
	ctx context.Context,
	filename string,
	category string,
) (int64, error) {

	var id int64

	query := `
		INSERT INTO documents (filename, category)
		VALUES ($1, $2)
		RETURNING id
	`

	err := r.db.QueryRow(
		ctx,
		query,
		filename,
		category,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("create document: %w", err)
	}

	return id, nil
}

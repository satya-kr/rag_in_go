package document

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Document is a row from the documents table.
type Document struct {
	ID         int64     `json:"id"`
	Filename   string    `json:"filename"`
	Category   string    `json:"category"`
	ChunkCount int       `json:"chunk_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// Create inserts a new document and returns its ID.
func (r *Repository) Create(
	ctx context.Context,
	filename string,
	category string,
) (int64, error) {

	var id int64

	err := r.db.QueryRow(
		ctx,
		`INSERT INTO documents (filename, category)
		 VALUES ($1, $2)
		 RETURNING id`,
		filename,
		category,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("create document: %w", err)
	}

	return id, nil
}

// List returns all documents with their chunk counts, newest first.
func (r *Repository) List(ctx context.Context) ([]Document, error) {

	rows, err := r.db.Query(
		ctx,
		`SELECT
			d.id,
			d.filename,
			d.category,
			COUNT(dc.id) AS chunk_count,
			d.created_at
		 FROM documents d
		 LEFT JOIN document_chunks dc ON dc.document_id = d.id
		 GROUP BY d.id
		 ORDER BY d.created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []Document

	for rows.Next() {
		var doc Document
		if err := rows.Scan(
			&doc.ID,
			&doc.Filename,
			&doc.Category,
			&doc.ChunkCount,
			&doc.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}

	return docs, nil
}

// Delete removes a document and all its chunks (cascade expected in schema).
func (r *Repository) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("document %d not found", id)
	}
	return nil
}

// DeleteBulk removes multiple documents and their chunks in one query.
func (r *Repository) DeleteBulk(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// Build $1,$2,... placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf("DELETE FROM documents WHERE id IN (%s)",
		strings.Join(placeholders, ","))
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk delete documents: %w", err)
	}
	return tag.RowsAffected(), nil
}

package documentchunk

import (
	"context"
	"fmt"
	"strings"

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

// Chunk represents a chunk returned from similarity search.
type Chunk struct {
	ID         int64
	DocumentID int64
	ChunkIndex int
	Content    string
	Distance   float32
}

func (r *Repository) Create(
	ctx context.Context,
	documentID int64,
	chunkIndex int,
	content string,
	embedding []float32,
) (int64, error) {

	// Convert []float32 to pgvector format:
	// [0.123,0.456,0.789,...]
	vector := formatVector(embedding)

	var id int64

	query := `
		INSERT INTO document_chunks (
			document_id,
			chunk_index,
			content,
			embedding
		)
		VALUES ($1, $2, $3, $4::vector)
		RETURNING id
	`

	err := r.db.QueryRow(
		ctx,
		query,
		documentID,
		chunkIndex,
		content,
		vector,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("create document chunk: %w", err)
	}

	return id, nil
}

// Search performs vector similarity search.
func (r *Repository) Search(
	ctx context.Context,
	embedding []float32,
	limit int,
) ([]Chunk, error) {

	vector := formatVector(embedding)

	query := `
		SELECT
			id,
			document_id,
			chunk_index,
			content,
			embedding <=> $1::vector AS distance
		FROM document_chunks
		ORDER BY embedding <=> $1::vector
		LIMIT $2
	`

	rows, err := r.db.Query(
		ctx,
		query,
		vector,
		limit,
	)

	if err != nil {
		return nil, fmt.Errorf("search document chunks: %w", err)
	}

	defer rows.Close()

	var chunks []Chunk

	for rows.Next() {

		var chunk Chunk

		err := rows.Scan(
			&chunk.ID,
			&chunk.DocumentID,
			&chunk.ChunkIndex,
			&chunk.Content,
			&chunk.Distance,
		)

		if err != nil {
			return nil, fmt.Errorf("scan document chunk: %w", err)
		}

		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document chunks: %w", err)
	}

	return chunks, nil
}

func (r *Repository) SearchByCategory(
	ctx context.Context,
	embedding []float32,
	category string,
	limit int,
) ([]Chunk, error) {
	vector := formatVector(embedding)

	query := `
        SELECT
            dc.id,
            dc.document_id,
            dc.chunk_index,
            dc.content,
            dc.embedding <=> $1::vector AS distance
        FROM document_chunks dc
        JOIN documents d
            ON d.id = dc.document_id
        WHERE d.category = $2
        ORDER BY dc.embedding <=> $1::vector
        LIMIT $3
    `

	rows, err := r.db.Query(
		ctx,
		query,
		vector,
		category,
		limit,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"search document chunks by category: %w",
			err,
		)
	}

	defer rows.Close()

	var chunks []Chunk

	for rows.Next() {

		var chunk Chunk

		err := rows.Scan(
			&chunk.ID,
			&chunk.DocumentID,
			&chunk.ChunkIndex,
			&chunk.Content,
			&chunk.Distance,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"scan document chunk: %w",
				err,
			)
		}

		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate document chunks: %w",
			err,
		)
	}

	return chunks, nil
}

func formatVector(values []float32) string {
	parts := make([]string, len(values))

	for i, value := range values {
		parts[i] = fmt.Sprintf("%f", value)
	}

	return "[" + strings.Join(parts, ",") + "]"
}

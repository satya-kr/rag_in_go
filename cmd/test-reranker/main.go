package main

import (
	"context"
	"fmt"
	"go_rag/internal/reranker"
	"log"
)

func main() {

	documents := []reranker.Document{
		{
			ID:      1,
			Content: "Employees receive 20 days of annual leave.",
		},
		{
			ID:      2,
			Content: "Employees can request leave through the HR portal.",
		},
		{
			ID:      3,
			Content: "The company provides twelve public holidays every year.",
		},
		{
			ID:      4,
			Content: "Emergency leave can be requested from the manager.",
		},
	}

	// r := reranker.NewMockReranker()
	r := reranker.NewHTTPReranker(
		"http://localhost:8081",
	)

	results, err := r.Rerank(
		context.Background(),
		"How many annual leave days do employees get?",
		documents,
		3,
	)

	if err != nil {
		log.Fatal(err)
	}

	for _, doc := range results {
		fmt.Printf(
			"ID: %d | Rerank Score: %.2f | %s\n",
			doc.ID,
			doc.RerankScore,
			doc.Content,
		)
	}
}

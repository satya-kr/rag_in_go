package main

import (
	"context"
	"fmt"
	"go_rag/internal/reranker"
	"log"
)

func main() {

	r, err := reranker.NewCohereRerankerFromEnv()
	if err != nil {
		log.Fatal(err)
	}

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
	}

	query := "How many annual leave days do employees get?"

	results, err := r.Rerank(
		context.Background(),
		query,
		documents,
		3,
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("======================================")
	fmt.Println("COHERE RERANKING")
	fmt.Println("======================================")

	for _, doc := range results {
		fmt.Printf(
			"ID: %d | Retrieval: %.4f | Rerank: %.4f | %s\n",
			doc.ID,
			doc.RetrievalScore,
			doc.RerankScore,
			doc.Content,
		)
	}
}

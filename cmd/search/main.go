package main

import (
	"context"
	"fmt"
	"go_rag/internal/database"
	"go_rag/internal/documentchunk"
	"go_rag/internal/embedding"
	"go_rag/internal/llm"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {

	ctx := context.Background()

	// -----------------------------------------
	// LOAD ENV
	// -----------------------------------------

	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	// -----------------------------------------
	// DATABASE
	// -----------------------------------------

	db, err := database.New(ctx, databaseURL)

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	fmt.Println("Database connected")

	// -----------------------------------------
	// EMBEDDING CLIENT
	// -----------------------------------------

	client := embedding.NewClient()

	// -----------------------------------------
	// QUESTION
	// -----------------------------------------

	// question := "What is VSCode"
	question := "What is RAG?"

	fmt.Println("Question:", question)

	// -----------------------------------------
	// CREATE QUESTION EMBEDDING
	// -----------------------------------------

	questionEmbedding, err := client.CreateEmbedding(
		ctx,
		question,
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(
		"Question embedding dimensions:",
		len(questionEmbedding),
	)

	// -----------------------------------------
	// VALIDATE EMBEDDING
	// -----------------------------------------

	if len(questionEmbedding) != 768 {
		log.Fatalf(
			"invalid embedding dimension: got %d, expected 768",
			len(questionEmbedding),
		)
	}

	// -----------------------------------------
	// CHUNK REPOSITORY
	// -----------------------------------------

	chunkRepo := documentchunk.NewRepository(db.Pool)

	// -----------------------------------------
	// SIMILARITY SEARCH
	// -----------------------------------------

	// results, err := chunkRepo.Search(
	// 	ctx,
	// 	questionEmbedding,
	// 	5,
	// )

	// OR Similarity Search + category
	results, err := chunkRepo.SearchByCategory(
		ctx,
		questionEmbedding,
		"technology",
		5,
	)

	if err != nil {
		log.Fatal(err)
	}

	// -----------------------------------------
	// PRINT RESULTS
	// -----------------------------------------

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("TOP 5 SIMILAR CHUNKS")
	fmt.Println("======================================")

	for i, chunk := range results {

		fmt.Println()
		fmt.Printf("Result #%d\n", i+1)
		fmt.Println("--------------------------------------")

		fmt.Println("Chunk ID:", chunk.ID)
		fmt.Println("Document ID:", chunk.DocumentID)
		fmt.Println("Chunk Index:", chunk.ChunkIndex)
		fmt.Printf("Distance: %.6f\n", chunk.Distance)

		fmt.Println("Content:")
		fmt.Println(chunk.Content)
	}

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("SEARCH COMPLETED")
	fmt.Println("======================================")

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("Generating answer using LLM...")
	fmt.Println("======================================")

	// Build context
	contextText := ""

	for i, chunk := range results {

		contextText += fmt.Sprintf(
			"\n[Source %d]\n%s\n",
			i+1,
			chunk.Content,
		)
	}

	prompt := fmt.Sprintf(`
	You are a helpful AI assistant.

	Answer the user's question using ONLY the context provided below.

	If the answer cannot be found in the context, say:
	"I don't know based on the provided documents."

	Do not use your own knowledge.

	Context:
	%s

	Question:
	%s

	Answer:
	`, contextText, question)

	llmClient := llm.NewClient()

	answer, err := llmClient.Generate(
		ctx,
		prompt,
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("FINAL RAG ANSWER")
	fmt.Println("======================================")
	fmt.Println(answer)
}

package main

import (
	"context"
	"fmt"
	"go_rag/internal/chunker"
	"go_rag/internal/database"
	"go_rag/internal/document"
	"go_rag/internal/documentchunk"
	"go_rag/internal/embedding"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Document struct {
	ID       string
	Content  string
	Metadata Metadata
}

type Metadata struct {
	Source string
	DocID  string
}

var myText = `Retrieval-augmented generation (RAG) is a technique that enables large language models (LLMs) to retrieve and incorporate new information from external data sources.[1] With RAG, LLMs first refer to a specified set of documents, then respond to user queries. These documents supplement information from the LLM's pre-existing training data.[2] This allows LLMs to use domain-specific and/or updated information that is not available in the training data.[2] For example, this enables LLM-based chatbots to access internal company data or generate responses based on authoritative sources. The technique was first proposed in 2020 and has since become a widely adopted approach in modern AI systems.

RAG improves LLMs by incorporating information retrieval before generating responses.[3] Unlike LLMs that rely on static training data, RAG pulls relevant text from databases, uploaded documents, or web sources.[1] According to Ars Technica, "RAG is a way of improving LLM performance, in essence by blending the LLM process with a web search or other document look-up process to help LLMs stick to the facts." This method helps reduce AI hallucinations,[3] which have caused chatbots to describe policies that don't exist, or recommend nonexistent legal cases to lawyers that are looking for citations to support their arguments.[4]

RAG also reduces the need to retrain LLMs with new data, saving on computational and financial costs.[1] Beyond efficiency gains, RAG also allows LLMs to include sources in their responses, so users can verify the cited sources. This provides greater transparency, as users can cross-check retrieved content to ensure accuracy and relevance.

The term retrieval-augmented generation (RAG) was introduced in a 2020 paper that described combining a parametric language model with a non-parametric external memory accessed through retrieval at inference time.[3]`

func main() {

	err := godotenv.Load()
	ctx := context.Background()
	if err != nil {
		log.Println("No .env file found")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	// --------------------------------------------------
	// DATABASE
	// --------------------------------------------------
	db, err := database.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var result int
	err = db.Pool.QueryRow(
		ctx,
		"SELECT 1",
	).Scan(&result)

	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Database query successful:", result)

	// --------------------------------------------------
	// DOCUMENT
	// --------------------------------------------------
	documentRepo := document.NewRepository(db.Pool)
	documentID, err := documentRepo.Create(
		ctx,
		"rag-example.txt",
		"technology",
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Document created:", documentID)
	// --------------------------------------------------
	// CHUNK REPOSITORY
	// --------------------------------------------------
	chunkRepo := documentchunk.NewRepository(db.Pool)

	// text := "What is RAG?"

	// --------------------------------------------------
	// EMBEDDING CLIENT
	// --------------------------------------------------
	client := embedding.NewClient()
	// --------------------------------------------------
	// CHUNKING
	// --------------------------------------------------
	chunks := chunker.SplitText(myText, 100, 20)
	fmt.Println("Total chunks:", len(chunks))

	// --------------------------------------------------
	// EMBEDDING + SAVE CHUNKS
	// --------------------------------------------------
	for i, chunk := range chunks {
		fmt.Printf("Processing chunk %d...\n", i)

		// Create 768D embedding
		vector, err := client.CreateEmbedding(ctx, chunk)
		if err != nil {
			log.Fatal(err)
		}

		// Validate embedding dimension
		if len(vector) != 768 {
			log.Fatalf(
				"invalid embedding dimension: got %d, expected 768",
				len(vector),
			)
		}

		// Save chunk + embedding
		chunkID, err := chunkRepo.Create(
			ctx,
			documentID,
			i,
			chunk,
			vector,
		)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf(
			"Chunk %d saved → ID=%d, dimensions=%d\n",
			i,
			chunkID,
			len(vector),
		)
	}
	fmt.Println("\n================================")
	fmt.Println("RAG ingestion completed")
	fmt.Println("================================")

	// fmt.Println("Dimensions:", len(vector))
	// fmt.Println("Embedding:", vector)
}

package main

import (
	"context"
	"fmt"
	"go_rag/internal/database"
	"go_rag/internal/documentchunk"
	"go_rag/internal/embedding"
	"go_rag/internal/llm"
	"go_rag/internal/reranker"
	"go_rag/internal/retrieval"
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
	// question := "What is RAG?"
	question := "What is retrieval augmented generation?"
	// question := "What is the capital of France?"

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
		10,
	)

	if err != nil {
		log.Fatal(err)
	}

	// -----------------------------------------
	// KEYWORD SEARCH
	// -----------------------------------------

	keywordResults, err := chunkRepo.KeywordSearchByCategory(
		ctx,
		question,
		"technology",
		10,
	)

	if err != nil {
		log.Fatal(err)
	}

	hybridResults := retrieval.RRF(
		results,
		keywordResults,
		60,
		10,
	)

	// -----------------------------------------
	// COHERE RERANKING
	// -----------------------------------------

	cohereReranker, err := reranker.NewCohereRerankerFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	rerankDocuments := make([]reranker.Document, 0, len(hybridResults))

	for _, result := range hybridResults {
		rerankDocuments = append(
			rerankDocuments,
			reranker.Document{
				ID:             result.Chunk.ID,
				Content:        result.Chunk.Content,
				RetrievalScore: result.RRFScore,
			},
		)
	}

	rerankedResults, err := cohereReranker.Rerank(
		ctx,
		question,
		rerankDocuments,
		5,
	)

	if err != nil {
		log.Fatal(err)
	}

	const rerankThreshold = 0.40

	if len(rerankedResults) == 0 ||
		rerankedResults[0].RerankScore < rerankThreshold {

		fmt.Println()
		fmt.Println("======================================")
		fmt.Println("LOW RELEVANCE")
		fmt.Println("======================================")
		fmt.Println("I don't know based on the provided documents.")

		return
	}

	// fmt.Println()
	// fmt.Println("======================================")
	// fmt.Println("TOP KEYWORD SEARCH RESULTS")
	// fmt.Println("======================================")

	// for i, chunk := range keywordResults {

	// 	fmt.Println()
	// 	fmt.Printf("Result #%d\n", i+1)
	// 	fmt.Println("--------------------------------------")

	// 	fmt.Println("Chunk ID:", chunk.ID)
	// 	fmt.Println("Document ID:", chunk.DocumentID)
	// 	fmt.Println("Chunk Index:", chunk.ChunkIndex)

	// 	fmt.Printf(
	// 		"Keyword Score: %.6f\n",
	// 		chunk.KeywordScore,
	// 	)

	// 	fmt.Println("Content:")
	// 	fmt.Println(chunk.Content)
	// }

	// -----------------------------------------
	// PRINT RESULTS
	// -----------------------------------------

	// fmt.Println()
	// fmt.Println("======================================")
	// fmt.Println("TOP 5 SIMILAR CHUNKS")
	// fmt.Println("======================================")

	// for i, chunk := range results {

	// 	fmt.Println()
	// 	fmt.Printf("Result #%d\n", i+1)
	// 	fmt.Println("--------------------------------------")

	// 	fmt.Println("Chunk ID:", chunk.ID)
	// 	fmt.Println("Document ID:", chunk.DocumentID)
	// 	fmt.Println("Chunk Index:", chunk.ChunkIndex)
	// 	fmt.Printf("Distance: %.6f\n", chunk.Distance)

	// 	fmt.Println("Content:")
	// 	fmt.Println(chunk.Content)
	// }

	// fmt.Println()
	// fmt.Println("======================================")
	// fmt.Println("SEARCH COMPLETED")
	// fmt.Println("======================================")

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("VECTOR RESULTS")
	fmt.Println("======================================")

	for i, chunk := range results {
		fmt.Printf(
			"Rank %d | ID %d | Distance %.6f\n",
			i+1,
			chunk.ID,
			chunk.Distance,
		)
	}

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("KEYWORD RESULTS")
	fmt.Println("======================================")

	for i, chunk := range keywordResults {
		fmt.Printf(
			"Rank %d | ID %d | Keyword Score %.6f\n",
			i+1,
			chunk.ID,
			chunk.KeywordScore,
		)
	}

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("HYBRID RESULTS - RRF")
	fmt.Println("======================================")

	for i, result := range hybridResults {

		fmt.Printf(
			"Rank %d | ID %d | RRF %.6f | Vector Rank %d | Keyword Rank %d\n",
			i+1,
			result.Chunk.ID,
			result.RRFScore,
			result.VectorRank,
			result.KeywordRank,
		)
	}

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("COHERE RERANKED RESULTS")
	fmt.Println("======================================")

	for i, doc := range rerankedResults {
		fmt.Printf(
			"Rank %d | ID %d | RRF %.6f | Cohere %.6f\n",
			i+1,
			doc.ID,
			doc.RetrievalScore,
			doc.RerankScore,
		)

		fmt.Println("Content:", doc.Content)
		fmt.Println()
	}

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("Generating answer using LLM...")
	fmt.Println("======================================")

	// Build context
	contextText := ""

	// for i, chunk := range results {
	for i, doc := range rerankedResults {

		contextText += fmt.Sprintf(
			"\n[Source %d]\n%s\n",
			i+1,
			// chunk.Content,
			doc.Content,
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

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("CONTEXT SENT TO LLM")
	fmt.Println("======================================")
	fmt.Println(contextText)

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("PROMPT SENT TO LLM")
	fmt.Println("======================================")
	fmt.Println(prompt)

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

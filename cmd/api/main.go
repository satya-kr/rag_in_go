package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go_rag/internal/database"
	"go_rag/internal/documentchunk"
	"go_rag/internal/embedding"
	"go_rag/internal/llm"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type queryRequest struct {
	Question string `json:"question"`
}

type queryResponse struct {
	Answer string `json:"answer"`
	Error  string `json:"error,omitempty"`
}

func main() {
	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := database.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	chunkRepo := documentchunk.NewRepository(db.Pool)
	embClient := embedding.NewClient()
	llmClient := llm.NewClient()

	http.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req queryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Question == "" {
			json.NewEncoder(w).Encode(queryResponse{Error: "invalid request"})
			return
		}

		qEmbedding, err := embClient.CreateEmbedding(r.Context(), req.Question)
		if err != nil {
			json.NewEncoder(w).Encode(queryResponse{Error: err.Error()})
			return
		}

		chunks, err := chunkRepo.Search(r.Context(), qEmbedding, 5)
		if err != nil {
			json.NewEncoder(w).Encode(queryResponse{Error: err.Error()})
			return
		}

		contextText := ""
		for i, c := range chunks {
			contextText += fmt.Sprintf("\n[Source %d]\n%s\n", i+1, c.Content)
		}

		prompt := fmt.Sprintf(`You are a helpful AI assistant.
		Answer the user's question using ONLY the context provided below.
		If the answer cannot be found in the context, say: "I don't know based on the provided documents."
		Do not use your own knowledge.

		Context:
		%s

		Question:
		%s

		Answer:`, contextText, req.Question)

		answer, err := llmClient.Generate(r.Context(), prompt)
		if err != nil {
			json.NewEncoder(w).Encode(queryResponse{Error: err.Error()})
			return
		}

		json.NewEncoder(w).Encode(queryResponse{Answer: answer})
	})

	log.Println("API server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

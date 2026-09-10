package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type Request struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type Result struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

type Response struct {
	Results []Result `json:"results"`
}

// sanitizeLog removes newlines to prevent log injection.
func sanitizeLog(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func rerankHandler(w http.ResponseWriter, r *http.Request) {

	var req Request

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Query: %s", sanitizeLog(req.Query))

	for i, doc := range req.Documents {
		log.Printf("Document %d: %s", i, sanitizeLog(doc))
	}

	// Fake scores for testing the HTTP client.
	results := []Result{
		{Index: 0, Score: 0.98},
		{Index: 1, Score: 0.25},
		{Index: 2, Score: 0.10},
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(Response{Results: results}); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func main() {

	http.HandleFunc("/rerank", rerankHandler)

	log.Println("Reranker test server running on http://localhost:8081")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}

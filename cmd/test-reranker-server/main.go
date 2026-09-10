package main

import (
	"encoding/json"
	"log"
	"net/http"
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

func rerankHandler(w http.ResponseWriter, r *http.Request) {

	var req Request

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Query: %s", req.Query)

	for i, doc := range req.Documents {
		log.Printf("Document %d: %s", i, doc)
	}

	// Fake scores for testing the HTTP client.
	results := []Result{
		{Index: 0, Score: 0.98},
		{Index: 1, Score: 0.25},
		{Index: 2, Score: 0.10},
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(Response{
		Results: results,
	})
}

func main() {

	http.HandleFunc("/rerank", rerankHandler)

	log.Println("Reranker test server running on http://localhost:8081")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}

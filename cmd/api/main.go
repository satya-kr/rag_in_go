package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go_rag/internal/chunker"
	"go_rag/internal/database"
	"go_rag/internal/document"
	"go_rag/internal/documentchunk"
	"go_rag/internal/embedding"
	"go_rag/internal/extractor"
	"go_rag/internal/llm"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// -----------------------------------------
// REQUEST / RESPONSE TYPES
// -----------------------------------------

type queryRequest struct {
	Question string `json:"question"`
	Model    string `json:"model,omitempty"`
}

type queryResponse struct {
	Answer string `json:"answer"`
	Error  string `json:"error,omitempty"`
}

type fileResult struct {
	Filename string `json:"filename"`
	Chunks   int    `json:"chunks"`
	Error    string `json:"error,omitempty"`
}

type uploadResponse struct {
	Files []fileResult `json:"files"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type bulkDeleteRequest struct {
	IDs []int64 `json:"ids"`
}

// -----------------------------------------
// HELPERS
// -----------------------------------------

// stage logs a labelled step with elapsed time since start.
func stage(start time.Time, format string, args ...any) {
	elapsed := time.Since(start)
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%6.0fms] %s", float64(elapsed.Milliseconds()), msg)
}

// cors sets permissive CORS headers and handles OPTIONS preflight.
// Returns true if the request was a preflight (caller should return).
func cors(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// writeJSON writes status + JSON body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode: %v", err)
	}
}

// -----------------------------------------
// INGESTION
// -----------------------------------------

// ingestFile extracts text from a multipart file, chunks it, embeds each
// chunk, and persists everything to the database.
// Returns the number of chunks created.
func ingestFile(
	ctx context.Context,
	fh *multipart.FileHeader,
	docRepo *document.Repository,
	chunkRepo *documentchunk.Repository,
	embClient *embedding.Client,
	category string,
) (int, error) {

	start := time.Now()
	log.Printf("──────────────────────────────────────────")
	log.Printf("[INGEST] START  file=%q  size=%d bytes  category=%q",
		fh.Filename, fh.Size, category)

	f, err := fh.Open()
	if err != nil {
		return 0, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// -----------------------------------------
	// TEXT EXTRACTION
	// -----------------------------------------

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	stage(start, "[INGEST] EXTRACT  type=%s", ext)

	var text string

	switch ext {
	case ".pdf":
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			return 0, fmt.Errorf("read pdf bytes: %w", readErr)
		}
		stage(start, "[INGEST] EXTRACT  read %d bytes from PDF", len(data))
		var pdfErr error
		text, pdfErr = extractor.FromPDFLenient(data)
		if pdfErr != nil {
			return 0, pdfErr
		}

	case ".txt", ".md", ".rst", "":
		var txtErr error
		text, txtErr = extractor.FromText(f)
		if txtErr != nil {
			return 0, txtErr
		}

	default:
		return 0, fmt.Errorf("unsupported file type %q (supported: .txt, .md, .rst, .pdf)", ext)
	}

	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("file contains no extractable text")
	}

	stage(start, "[INGEST] EXTRACT  done  chars=%d", len(text))

	// -----------------------------------------
	// SAVE DOCUMENT RECORD
	// -----------------------------------------

	stage(start, "[INGEST] DB  saving document record")
	docID, err := docRepo.Create(ctx, fh.Filename, category)
	if err != nil {
		return 0, fmt.Errorf("create document record: %w", err)
	}
	stage(start, "[INGEST] DB  document saved  id=%d", docID)

	// -----------------------------------------
	// CHUNKING
	// -----------------------------------------

	stage(start, "[INGEST] CHUNK  splitting text  chunkSize=500 overlap=50")
	chunks := chunker.SplitText(text, 500, 50)
	stage(start, "[INGEST] CHUNK  done  total_chunks=%d", len(chunks))

	// -----------------------------------------
	// EMBED + SAVE CHUNKS
	// -----------------------------------------

	for i, chunk := range chunks {
		stage(start, "[INGEST] EMBED  chunk %d/%d  chars=%d  → calling Ollama...",
			i+1, len(chunks), len(chunk))

		vector, err := embClient.CreateEmbedding(ctx, chunk)
		if err != nil {
			return i, fmt.Errorf("embed chunk %d: %w", i, err)
		}

		stage(start, "[INGEST] EMBED  chunk %d/%d  done  dims=%d  → saving to DB...",
			i+1, len(chunks), len(vector))

		if _, err := chunkRepo.Create(ctx, docID, i, chunk, vector); err != nil {
			return i, fmt.Errorf("save chunk %d: %w", i, err)
		}

		stage(start, "[INGEST] DB  chunk %d/%d saved", i+1, len(chunks))
	}

	stage(start, "[INGEST] DONE  file=%q  chunks=%d  total_time=%s",
		fh.Filename, len(chunks), time.Since(start).Round(time.Millisecond))
	log.Printf("──────────────────────────────────────────")

	return len(chunks), nil
}

// -----------------------------------------
// HANDLERS
// -----------------------------------------

// handleQuery answers a question using RAG (vector similarity + LLM).
func handleQuery(
	chunkRepo *documentchunk.Repository,
	embClient *embedding.Client,
	llmClient *llm.Client,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}

		start := time.Now()
		log.Printf("══════════════════════════════════════════")
		log.Printf("[QUERY] START  remote=%s", r.RemoteAddr)

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, queryResponse{Error: "request body too large or unreadable"})
			return
		}

		var req queryRequest
		if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Question) == "" {
			writeJSON(w, http.StatusBadRequest, queryResponse{Error: "invalid request: question is required"})
			return
		}

		// Allow per-request model override from the UI
		if req.Model != "" {
			llmClient.Model = req.Model
		}

		stage(start, "[QUERY] QUESTION  %q", req.Question)

		// -----------------------------------------
		// EMBED QUESTION
		// -----------------------------------------

		stage(start, "[QUERY] EMBED  embedding question → calling Ollama nomic-embed-text...")
		qEmbedding, err := embClient.CreateEmbedding(r.Context(), req.Question)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, queryResponse{Error: err.Error()})
			return
		}
		stage(start, "[QUERY] EMBED  done  dims=%d", len(qEmbedding))

		// -----------------------------------------
		// VECTOR SEARCH
		// -----------------------------------------

		stage(start, "[QUERY] SEARCH  running vector similarity search  top_k=5...")
		chunks, err := chunkRepo.Search(r.Context(), qEmbedding, 5)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, queryResponse{Error: err.Error()})
			return
		}
		stage(start, "[QUERY] SEARCH  done  results=%d", len(chunks))

		for i, c := range chunks {
			stage(start, "[QUERY] SEARCH  rank=%d  chunk_id=%d  dist=%.4f  preview=%q",
				i+1, c.ID, c.Distance, truncate(c.Content, 60))
		}

		// -----------------------------------------
		// BUILD CONTEXT + PROMPT
		// -----------------------------------------

		stage(start, "[QUERY] PROMPT  building context from %d chunks", len(chunks))
		contextText := ""
		for i, c := range chunks {
			contextText += fmt.Sprintf("\n[Source %d]\n%s\n", i+1, c.Content)
		}

		prompt := fmt.Sprintf(`You are a precise technical assistant. Answer ONLY using the context below. Be direct and concise.

RULES:
- Answer only from the context. Do not use outside knowledge.
- If the answer is not in the context, respond: "I don't know based on the provided documents."
- Do not explain your reasoning. Do not repeat the question. Just answer.

CONTEXT:
%s

QUESTION: %s

ANSWER:`, contextText, req.Question)

		stage(start, "[QUERY] PROMPT  done  prompt_chars=%d", len(prompt))

		// -----------------------------------------
		// LLM GENERATION
		// -----------------------------------------

		stage(start, "[QUERY] LLM  sending prompt to %s → waiting for response...", llmClient.Model)
		answer, err := llmClient.Generate(r.Context(), prompt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, queryResponse{Error: err.Error()})
			return
		}
		stage(start, "[QUERY] LLM  done  answer_chars=%d", len(answer))

		// -----------------------------------------
		// DONE
		// -----------------------------------------

		stage(start, "[QUERY] DONE  total_time=%s", time.Since(start).Round(time.Millisecond))
		log.Printf("══════════════════════════════════════════")

		writeJSON(w, http.StatusOK, queryResponse{Answer: answer})
	}
}

// handleUpload accepts multipart/form-data with one or more files.
func handleUpload(
	docRepo *document.Repository,
	chunkRepo *documentchunk.Repository,
	embClient *embedding.Client,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}

		start := time.Now()
		log.Printf("══════════════════════════════════════════")
		log.Printf("[UPLOAD] START  remote=%s", r.RemoteAddr)

		// Keep up to 32 MB per file part in memory; larger parts spill to
		// temp files automatically — this keeps RAM bounded for large uploads.
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "failed to parse multipart form"})
			return
		}
		defer r.MultipartForm.RemoveAll()

		category := strings.TrimSpace(r.FormValue("category"))
		if category == "" {
			category = "general"
		}

		fileHeaders := r.MultipartForm.File["files"]
		if len(fileHeaders) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no files provided (field name must be \"files\")"})
			return
		}

		stage(start, "[UPLOAD] FILES  count=%d  category=%q", len(fileHeaders), category)
		for i, fh := range fileHeaders {
			stage(start, "[UPLOAD] FILES  [%d] %q  %d bytes", i+1, fh.Filename, fh.Size)
		}

		results := make([]fileResult, 0, len(fileHeaders))

		for _, fh := range fileHeaders {
			res := fileResult{Filename: fh.Filename}

			n, err := ingestFile(r.Context(), fh, docRepo, chunkRepo, embClient, category)
			if err != nil {
				res.Error = err.Error()
				log.Printf("[UPLOAD] ERROR  file=%q  err=%v", fh.Filename, err)
			} else {
				res.Chunks = n
				log.Printf("[UPLOAD] OK  file=%q  chunks=%d", fh.Filename, n)
			}

			results = append(results, res)
		}

		stage(start, "[UPLOAD] DONE  files=%d  total_time=%s",
			len(fileHeaders), time.Since(start).Round(time.Millisecond))
		log.Printf("══════════════════════════════════════════")

		writeJSON(w, http.StatusOK, uploadResponse{Files: results})
	}
}

// handleDocuments lists all documents (GET) or deletes one (DELETE /documents/{id}).
func handleDocuments(docRepo *document.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}

		idStr := strings.TrimPrefix(r.URL.Path, "/documents")
		idStr = strings.TrimPrefix(idStr, "/")

		switch r.Method {

		case http.MethodGet:
			log.Printf("[DOCS] LIST  fetching all documents...")
			docs, err := docRepo.List(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			if docs == nil {
				docs = []document.Document{}
			}
			log.Printf("[DOCS] LIST  done  count=%d", len(docs))
			writeJSON(w, http.StatusOK, docs)

		case http.MethodDelete:
			if idStr == "" {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "document id required: DELETE /documents/{id}"})
				return
			}
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid document id"})
				return
			}
			log.Printf("[DOCS] DELETE  id=%d", id)
			if err := docRepo.Delete(r.Context(), id); err != nil {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
				return
			}
			log.Printf("[DOCS] DELETE  done  id=%d", id)
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		}
	}
}

// handleBulkDelete deletes multiple documents by ID in one request.
func handleBulkDelete(docRepo *document.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}
		var req bulkDeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "ids array required"})
			return
		}
		log.Printf("[DOCS] BULK DELETE  ids=%v", req.IDs)
		n, err := docRepo.DeleteBulk(r.Context(), req.IDs)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		log.Printf("[DOCS] BULK DELETE  done  deleted=%d", n)
		writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
	}
}

// truncate shortens s to max runes for log previews.
func truncate(s string, max int) string {
	runes := []rune(s)
	s = strings.ReplaceAll(s, "\n", " ")
	runes = []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// -----------------------------------------
// MAIN
// -----------------------------------------

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Printf("══════════════════════════════════════════")
	log.Printf("[BOOT] RAG API server starting...")

	// -----------------------------------------
	// LOAD ENV
	// -----------------------------------------

	for _, p := range []string{".env", "../../.env"} {
		if err := godotenv.Load(p); err == nil {
			log.Printf("[BOOT] ENV  loaded from %q", p)
			break
		}
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("[BOOT] ENV  DATABASE_URL is not set")
	}

	llmModel := os.Getenv("LLM_MODEL")
	if llmModel == "" {
		llmModel = "deepseek-r1:latest"
	}
	log.Printf("[BOOT] CONFIG  LLM_MODEL=%s  (set LLM_MODEL env to override)", llmModel)

	// -----------------------------------------
	// DATABASE
	// -----------------------------------------

	log.Printf("[BOOT] DB  connecting to postgres...")
	ctx := context.Background()
	db, err := database.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("[BOOT] DB  connection failed: %v", err)
	}
	defer db.Close()
	log.Printf("[BOOT] DB  connected OK")

	// -----------------------------------------
	// REPOSITORIES + CLIENTS
	// -----------------------------------------

	docRepo := document.NewRepository(db.Pool)
	chunkRepo := documentchunk.NewRepository(db.Pool)
	embClient := embedding.NewClient()
	llmClient := llm.NewClient()

	log.Printf("[BOOT] CLIENTS  embedding_model=%s  llm_model=%s",
		embClient.Model, llmClient.Model)

	// -----------------------------------------
	// ROUTES
	// -----------------------------------------

	http.HandleFunc("/query", handleQuery(chunkRepo, embClient, llmClient))
	http.HandleFunc("/upload", handleUpload(docRepo, chunkRepo, embClient))
	http.HandleFunc("/documents", handleDocuments(docRepo))
	http.HandleFunc("/documents/", handleDocuments(docRepo))
	http.HandleFunc("/documents/bulk", handleBulkDelete(docRepo))

	log.Printf("[BOOT] ROUTES  /query  /upload  /documents  /documents/{id}  /documents/bulk")
	log.Printf("[BOOT] READY   listening on :8080")
	log.Printf("══════════════════════════════════════════")

	log.Fatal(http.ListenAndServe(":8080", nil))
}

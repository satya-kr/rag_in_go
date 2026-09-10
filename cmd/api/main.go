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

	"github.com/joho/godotenv"
)

// -----------------------------------------
// REQUEST / RESPONSE TYPES
// -----------------------------------------

type queryRequest struct {
	Question string `json:"question"`
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

// -----------------------------------------
// HELPERS
// -----------------------------------------

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

	f, err := fh.Open()
	if err != nil {
		return 0, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// -----------------------------------------
	// TEXT EXTRACTION
	// -----------------------------------------

	ext := strings.ToLower(filepath.Ext(fh.Filename))

	var text string

	switch ext {
	case ".pdf":
		// Read all bytes; extractor writes them to a temp file so pdf.Open
		// can seek correctly. FromPDFLenient skips unreadable pages (e.g.
		// scanned/image pages) rather than aborting the whole document.
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			return 0, fmt.Errorf("read pdf bytes: %w", readErr)
		}
		var pdfErr error
		text, pdfErr = extractor.FromPDFLenient(data)
		if pdfErr != nil {
			return 0, pdfErr
		}

	case ".txt", ".md", ".rst", "":
		// All plain-text formats — markdown, plain text, reStructuredText.
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

	// -----------------------------------------
	// SAVE DOCUMENT RECORD
	// -----------------------------------------

	docID, err := docRepo.Create(ctx, fh.Filename, category)
	if err != nil {
		return 0, fmt.Errorf("create document record: %w", err)
	}

	// -----------------------------------------
	// CHUNK → EMBED → SAVE
	// -----------------------------------------

	// chunkSize=500 chars, overlap=50 chars — good balance for large docs.
	chunks := chunker.SplitText(text, 500, 50)

	for i, chunk := range chunks {
		vector, err := embClient.CreateEmbedding(ctx, chunk)
		if err != nil {
			return i, fmt.Errorf("embed chunk %d: %w", i, err)
		}

		if _, err := chunkRepo.Create(ctx, docID, i, chunk, vector); err != nil {
			return i, fmt.Errorf("save chunk %d: %w", i, err)
		}
	}

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

		qEmbedding, err := embClient.CreateEmbedding(r.Context(), req.Question)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, queryResponse{Error: err.Error()})
			return
		}

		chunks, err := chunkRepo.Search(r.Context(), qEmbedding, 5)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, queryResponse{Error: err.Error()})
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
			writeJSON(w, http.StatusInternalServerError, queryResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, queryResponse{Answer: answer})
	}
}

// handleUpload accepts multipart/form-data with one or more files.
// Field name: "files" (multiple allowed). Optional field: "category".
// Each file is extracted, chunked, embedded, and saved independently —
// a failure on one file does not abort the others.
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

		results := make([]fileResult, 0, len(fileHeaders))

		for _, fh := range fileHeaders {
			res := fileResult{Filename: fh.Filename}

			n, err := ingestFile(r.Context(), fh, docRepo, chunkRepo, embClient, category)
			if err != nil {
				res.Error = err.Error()
				log.Printf("ingest %q: %v", fh.Filename, err)
			} else {
				res.Chunks = n
			}

			results = append(results, res)
		}

		writeJSON(w, http.StatusOK, uploadResponse{Files: results})
	}
}

// handleDocuments lists all documents (GET) or deletes one (DELETE /documents/{id}).
func handleDocuments(docRepo *document.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}

		// Extract optional trailing /{id} from the path.
		idStr := strings.TrimPrefix(r.URL.Path, "/documents")
		idStr = strings.TrimPrefix(idStr, "/")

		switch r.Method {

		case http.MethodGet:
			docs, err := docRepo.List(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			if docs == nil {
				docs = []document.Document{}
			}
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
			if err := docRepo.Delete(r.Context(), id); err != nil {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		}
	}
}

// -----------------------------------------
// MAIN
// -----------------------------------------

func main() {
	// -----------------------------------------
	// LOAD ENV
	// -----------------------------------------

	// Try loading .env from current dir, then two levels up (project root).
	// This allows running from both the project root and cmd/api/ directly.
	for _, p := range []string{".env", "../../.env"} {
		if err := godotenv.Load(p); err == nil {
			break
		}
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	// -----------------------------------------
	// DATABASE
	// -----------------------------------------

	ctx := context.Background()
	db, err := database.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// -----------------------------------------
	// REPOSITORIES + CLIENTS
	// -----------------------------------------

	docRepo := document.NewRepository(db.Pool)
	chunkRepo := documentchunk.NewRepository(db.Pool)
	embClient := embedding.NewClient()
	llmClient := llm.NewClient()

	// -----------------------------------------
	// ROUTES
	// -----------------------------------------

	http.HandleFunc("/query", handleQuery(chunkRepo, embClient, llmClient))
	http.HandleFunc("/upload", handleUpload(docRepo, chunkRepo, embClient))
	http.HandleFunc("/documents", handleDocuments(docRepo))
	http.HandleFunc("/documents/", handleDocuments(docRepo)) // catches /documents/{id}

	log.Println("API server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

import { useCallback, useEffect, useRef, useState } from "react";
import "./App.css";

const API = "http://localhost:8080";

// Supported upload types — checked case-insensitively.
const SUPPORTED_EXTS = new Set(["txt", "pdf", "md", "rst"]);

const MODEL_OPTIONS = [
  { group: "Ollama (local)", models: [
    { value: "deepseek-r1:latest",  label: "DeepSeek R1" },
    { value: "llama3.2:latest",     label: "Llama 3.2" },
    { value: "mistral:latest",      label: "Mistral" },
    { value: "gemma3:latest",       label: "Gemma 3" },
  ]},
  { group: "OpenAI", models: [
    { value: "gpt-4o",       label: "GPT-4o" },
    { value: "gpt-4o-mini",  label: "GPT-4o Mini" },
    { value: "gpt-4-turbo",  label: "GPT-4 Turbo" },
    { value: "gpt-3.5-turbo",label: "GPT-3.5 Turbo" },
  ]},
];

// ─── tiny helpers ────────────────────────────────────────────────────────────

function fileExt(name) {
  // Handles uppercase extensions like README.MD, report.PDF
  return name.split(".").pop().toLowerCase();
}

function fmtSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function fmtDate(iso) {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

// ─── Upload Panel ─────────────────────────────────────────────────────────────

function UploadPanel({ onUploaded }) {
  const [dragging, setDragging] = useState(false);
  const [files, setFiles] = useState([]); // { file, status, chunks, error }
  const [category, setCategory] = useState("general");
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef(null);

  const addFiles = useCallback((incoming) => {
    const valid = Array.from(incoming).filter((f) =>
      // Case-insensitive check so README.MD, report.PDF etc. are accepted
      SUPPORTED_EXTS.has(fileExt(f.name))
    );
    const rejected = Array.from(incoming).length - valid.length;
    if (rejected > 0) {
      // Non-blocking — just log; the file list shows only accepted files
      console.warn(`${rejected} file(s) skipped: unsupported type (use .txt, .md, .rst, .pdf)`);
    }
    setFiles((prev) => [
      ...prev,
      ...valid.map((f) => ({ file: f, status: "pending", chunks: 0, error: "" })),
    ]);
  }, []);

  const onDrop = useCallback(
    (e) => {
      e.preventDefault();
      setDragging(false);
      addFiles(e.dataTransfer.files);
    },
    [addFiles]
  );

  const removeFile = (idx) =>
    setFiles((prev) => prev.filter((_, i) => i !== idx));

  const clearDone = () =>
    setFiles((prev) => prev.filter((f) => f.status === "pending"));

  async function upload() {
    // Only send files that are still pending (not already done/errored)
    const pending = files.filter((f) => f.status === "pending");
    if (pending.length === 0 || uploading) return;
    setUploading(true);

    // Mark only pending files as uploading
    setFiles((prev) =>
      prev.map((f) => f.status === "pending" ? { ...f, status: "uploading" } : f)
    );

    const form = new FormData();
    form.append("category", category);
    pending.forEach((f) => form.append("files", f.file));

    try {
      const res = await fetch(`${API}/upload`, { method: "POST", body: form });
      const data = await res.json();

      // Map results back by index into the pending subset
      setFiles((prev) => {
        let resultIdx = 0;
        return prev.map((f) => {
          if (f.status !== "uploading") return f;
          const result = data.files?.[resultIdx++];
          if (!result) return { ...f, status: "error", error: "no response" };
          return result.error
            ? { ...f, status: "error", error: result.error }
            : { ...f, status: "done", chunks: result.chunks };
        });
      });

      onUploaded(); // refresh documents list
    } catch {
      setFiles((prev) =>
        prev.map((f) =>
          f.status === "uploading"
            ? { ...f, status: "error", error: "Network error" }
            : f
        )
      );
    } finally {
      setUploading(false);
    }
  }

  const hasPending = files.some((f) => f.status === "pending");
  const hasDone = files.some((f) => f.status === "done" || f.status === "error");

  return (
    <section className="panel upload-panel">
      <h2>Upload Documents</h2>

      {/* Category */}
      <div className="field-row">
        <label htmlFor="category">Category</label>
        <input
          id="category"
          value={category}
          onChange={(e) => setCategory(e.target.value)}
          placeholder="e.g. technology"
          disabled={uploading}
        />
      </div>

      {/* Drop zone */}
      <div
        className={`dropzone${dragging ? " dragging" : ""}`}
        onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        onClick={() => inputRef.current?.click()}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => e.key === "Enter" && inputRef.current?.click()}
        aria-label="Drop files here or click to browse"
      >
        <input
          ref={inputRef}
          type="file"
          multiple
          accept=".txt,.pdf,.md,.rst"
          style={{ display: "none" }}
          onChange={(e) => addFiles(e.target.files)}
        />
        <span className="drop-icon">📂</span>
        <p>Drop <strong>.txt</strong>, <strong>.pdf</strong>, <strong>.md</strong>, or <strong>.rst</strong> files here</p>
        <p className="drop-sub">or click to browse — mix types, multiple files supported</p>
      </div>

      {/* File list */}
      {files.length > 0 && (
        <ul className="file-list">
          {files.map((item, i) => (
            <li key={i} className={`file-item ${item.status}`}>
              <span className={`ext-badge ext-${fileExt(item.file.name)}`}>
                {fileExt(item.file.name).toUpperCase()}
              </span>
              <span className="file-name" title={item.file.name}>
                {item.file.name}
              </span>
              <span className="file-size">{fmtSize(item.file.size)}</span>

              {item.status === "uploading" && (
                <span className="file-status uploading">⏳ Uploading…</span>
              )}
              {item.status === "done" && (
                <span className="file-status done">✅ {item.chunks} chunks</span>
              )}
              {item.status === "error" && (
                <span className="file-status error" title={item.error}>
                  ❌ {item.error}
                </span>
              )}

              {item.status === "pending" && (
                <button
                  className="btn-icon"
                  onClick={(e) => { e.stopPropagation(); removeFile(i); }}
                  aria-label="Remove file"
                >
                  ✕
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {/* Actions */}
      <div className="upload-actions">
        {hasDone && (
          <button className="btn-secondary" onClick={clearDone} disabled={uploading}>
            Clear done
          </button>
        )}
        <button
          className="btn-primary"
          onClick={upload}
          disabled={!hasPending || uploading}
        >
          {uploading ? "Uploading…" : `Upload ${files.filter((f) => f.status === "pending").length || ""} file(s)`}
        </button>
      </div>
    </section>
  );
}

// ─── Documents Panel ──────────────────────────────────────────────────────────

function DocumentsPanel({ refresh }) {
  const [docs, setDocs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [deletingId, setDeletingId] = useState(null);
  const [selected, setSelected] = useState(new Set());
  const [bulkDeleting, setBulkDeleting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API}/documents`);
      const data = await res.json();
      setDocs(Array.isArray(data) ? data : []);
      setSelected(new Set());
    } catch {
      setDocs([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load, refresh]);

  async function deleteDoc(id) {
    if (!confirm("Delete this document and all its chunks?")) return;
    setDeletingId(id);
    try {
      await fetch(`${API}/documents/${id}`, { method: "DELETE" });
      setDocs((prev) => prev.filter((d) => d.id !== id));
      setSelected((prev) => { const s = new Set(prev); s.delete(id); return s; });
    } finally {
      setDeletingId(null);
    }
  }

  async function bulkDelete() {
    if (selected.size === 0) return;
    if (!confirm(`Delete ${selected.size} document(s) and all their chunks?`)) return;
    setBulkDeleting(true);
    try {
      await fetch(`${API}/documents/bulk`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ids: Array.from(selected) }),
      });
      setDocs((prev) => prev.filter((d) => !selected.has(d.id)));
      setSelected(new Set());
    } finally {
      setBulkDeleting(false);
    }
  }

  function toggleSelect(id) {
    setSelected((prev) => {
      const s = new Set(prev);
      s.has(id) ? s.delete(id) : s.add(id);
      return s;
    });
  }

  const allSelected = docs.length > 0 && selected.size === docs.length;

  function toggleAll() {
    setSelected(allSelected ? new Set() : new Set(docs.map((d) => d.id)));
  }

  return (
    <section className="panel docs-panel">
      <div className="panel-header">
        <h2>Ingested Documents</h2>
        <button className="btn-icon-sm" onClick={load} disabled={loading} aria-label="Refresh">
          🔄
        </button>
      </div>

      {loading && <p className="muted">Loading…</p>}

      {!loading && docs.length === 0 && (
        <p className="muted">No documents yet. Upload some files above.</p>
      )}

      {docs.length > 0 && (
        <>
          <div className="bulk-bar">
            <label className="select-all">
              <input
                type="checkbox"
                checked={allSelected}
                onChange={toggleAll}
              />
              {allSelected ? "Deselect all" : "Select all"}
            </label>
            {selected.size > 0 && (
              <button
                className="btn-delete-bulk"
                onClick={bulkDelete}
                disabled={bulkDeleting}
              >
                {bulkDeleting ? "Deleting…" : `🗑 Delete ${selected.size}`}
              </button>
            )}
          </div>

          <ul className="doc-list">
            {docs.map((doc) => (
              <li key={doc.id} className={`doc-item${selected.has(doc.id) ? " selected" : ""}`}>
                <input
                  type="checkbox"
                  className="doc-checkbox"
                  checked={selected.has(doc.id)}
                  onChange={() => toggleSelect(doc.id)}
                  aria-label={`Select ${doc.filename}`}
                />
                <div className="doc-info">
                  <span className={`ext-badge ext-${fileExt(doc.filename)}`}>
                    {fileExt(doc.filename).toUpperCase()}
                  </span>
                  <div className="doc-meta">
                    <span className="doc-name" title={doc.filename}>{doc.filename}</span>
                    <span className="doc-sub">
                      {doc.chunk_count} chunks · {doc.category} · {fmtDate(doc.created_at)}
                    </span>
                  </div>
                </div>
                <button
                  className="btn-delete"
                  onClick={() => deleteDoc(doc.id)}
                  disabled={deletingId === doc.id}
                  aria-label={`Delete ${doc.filename}`}
                >
                  {deletingId === doc.id ? "…" : "🗑"}
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

// ─── Chat Panel ───────────────────────────────────────────────────────────────

function ChatPanel() {
  const [question, setQuestion] = useState("");
  const [messages, setMessages] = useState([]);
  const [loading, setLoading] = useState(false);
  const [model, setModel] = useState("deepseek-r1:latest");
  const bottomRef = useRef(null);

  // Auto-scroll to latest message
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, loading]);

  async function handleSubmit(e) {
    e.preventDefault();
    const q = question.trim();
    if (!q || loading) return;

    setMessages((prev) => [...prev, { role: "user", text: q }]);
    setQuestion("");
    setLoading(true);

    try {
      const res = await fetch(`${API}/query`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ question: q, model }),
      });
      const data = await res.json();
      setMessages((prev) => [
        ...prev,
        { role: "assistant", text: data.answer || data.error || "No response." },
      ]);
    } catch {
      setMessages((prev) => [
        ...prev,
        { role: "assistant", text: "Failed to reach the RAG server." },
      ]);
    } finally {
      setLoading(false);
    }
  }

  function handleKeyDown(e) {
    // Submit on Enter, new line on Shift+Enter
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  }

  return (
    <section className="panel chat-panel">
      <div className="chat-header">
        <span className="chat-title">Neural Query Interface</span>
        <select
          className="model-select"
          value={model}
          onChange={(e) => setModel(e.target.value)}
          disabled={loading}
          aria-label="Select model"
        >
          {MODEL_OPTIONS.map((g) => (
            <optgroup key={g.group} label={g.group}>
              {g.models.map((m) => (
                <option key={m.value} value={m.value}>{m.label}</option>
              ))}
            </optgroup>
          ))}
        </select>
        <span className="chat-status">
          <span className="status-dot" />
          ONLINE
        </span>
      </div>

      <div className="chat-box">
        {messages.length === 0 && (
          <div className="chat-empty">
            <span className="chat-empty-icon">◈</span>
            <div>SYSTEM READY</div>
            <div>Upload documents, then query the knowledge base.</div>
          </div>
        )}

        {messages.map((m, i) => (
          <div key={i} className={`message ${m.role}`}>
            <span className="label">{m.role === "user" ? "You" : "AI"}</span>
            <p>{m.text}</p>
          </div>
        ))}

        {loading && (
          <div className="message assistant">
            <span className="label">AI</span>
            <p className="thinking">
              <span />
              <span />
              <span />
            </p>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      <form onSubmit={handleSubmit} className="input-row">
        <textarea
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ask something… (Enter to send, Shift+Enter for new line)"
          disabled={loading}
          rows={2}
        />
        <button type="submit" disabled={loading || !question.trim()}>
          Send
        </button>
      </form>
    </section>
  );
}

// ─── App ──────────────────────────────────────────────────────────────────────

export default function App() {
  // Incrementing this triggers DocumentsPanel to reload
  const [docRefresh, setDocRefresh] = useState(0);

  return (
    <div className="app-layout">
      {/* Left sidebar: upload + documents */}
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-icon">⬡</span>
          <span className="brand-name">RAG Studio</span>
          <span className="brand-tag">v1.0</span>
        </div>
        <UploadPanel onUploaded={() => setDocRefresh((n) => n + 1)} />
        <DocumentsPanel refresh={docRefresh} />
      </aside>

      {/* Main: chat */}
      <main className="main">
        <ChatPanel />
      </main>
    </div>
  );
}

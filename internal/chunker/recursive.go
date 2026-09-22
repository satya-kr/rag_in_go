package chunker

import (
	"strings"
	"unicode/utf8"
)

// defaultSeparators mirrors LangChain's RecursiveCharacterTextSplitter defaults.
var defaultSeparators = []string{"\n\n", "\n", " ", ""}

// SplitText splits text recursively using a separator hierarchy, then merges
// small splits into chunks of chunkSize with overlap — same behaviour as
// LangChain's RecursiveCharacterTextSplitter.
func SplitText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		return nil
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	splits := splitRecursive(text, defaultSeparators, chunkSize)
	return mergeSplits(splits, chunkSize, overlap)
}

// splitRecursive picks the first separator that actually appears in the text,
// splits on it, then recurses on any piece that is still too large.
func splitRecursive(text string, separators []string, chunkSize int) []string {
	if utf8.RuneCountInString(text) <= chunkSize {
		return []string{text}
	}

	// Find the first separator present in the text.
	sep := ""
	remaining := separators
	for i, s := range separators {
		if s == "" || strings.Contains(text, s) {
			sep = s
			remaining = separators[i+1:]
			break
		}
	}

	var splits []string
	var parts []string
	if sep == "" {
		// Character-level split as last resort.
		runes := []rune(text)
		for i := 0; i < len(runes); i += chunkSize {
			end := i + chunkSize
			if end > len(runes) {
				end = len(runes)
			}
			parts = append(parts, string(runes[i:end]))
		}
	} else {
		parts = strings.Split(text, sep)
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if utf8.RuneCountInString(part) <= chunkSize {
			splits = append(splits, part)
		} else {
			splits = append(splits, splitRecursive(part, remaining, chunkSize)...)
		}
	}
	return splits
}

// mergeSplits joins small splits into chunks up to chunkSize, carrying over
// overlap from the previous chunk — mirrors LangChain's _merge_splits.
func mergeSplits(splits []string, chunkSize, overlap int) []string {
	var chunks []string
	var current []string
	currentLen := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.Join(current, " "))
	}

	for _, s := range splits {
		sLen := utf8.RuneCountInString(s)

		// +1 for the space separator between pieces.
		extra := sLen
		if currentLen > 0 {
			extra++
		}

		if currentLen+extra > chunkSize && len(current) > 0 {
			flush()

			// Roll back current to maintain overlap.
			for currentLen > overlap && len(current) > 0 {
				removed := utf8.RuneCountInString(current[0])
				if currentLen > removed {
					currentLen -= removed + 1 // +1 for space
				} else {
					currentLen = 0
				}
				current = current[1:]
			}
		}

		current = append(current, s)
		if currentLen > 0 {
			currentLen++
		}
		currentLen += sLen
	}

	flush()
	return chunks
}

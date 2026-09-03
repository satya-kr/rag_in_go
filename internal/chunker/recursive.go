package chunker

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func SplitText(text string, chunkSize int, overlap int) []string {
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

	// Split into words.
	words := strings.Fields(text)

	var chunks []string
	fmt.Println(" =================== CHUNKING =================== ")
	fmt.Println("chunks ->>", chunks)

	start := 0

	for start < len(words) {
		var chunk []string
		charCount := 0

		end := start

		for end < len(words) {
			word := words[end]

			// +1 for space between words.
			extra := utf8.RuneCountInString(word)

			if len(chunk) > 0 {
				extra++
			}

			if charCount+extra > chunkSize {
				break
			}

			chunk = append(chunk, word)
			charCount += extra
			end++
		}

		// Handle a single word longer than chunkSize.
		if end == start {
			runes := []rune(words[start])

			if len(runes) > chunkSize {
				chunkText := string(runes[:chunkSize])
				chunks = append(chunks, chunkText)

				// Move forward while preserving overlap.
				start += chunkSize - overlap
				continue
			}
		}

		chunkText := strings.Join(chunk, " ")
		chunks = append(chunks, chunkText)

		if end >= len(words) {
			break
		}

		// Calculate overlap by characters.
		overlapChars := 0
		overlapWords := 0

		for i := end - 1; i >= start; i-- {
			wordLen := utf8.RuneCountInString(words[i])

			if overlapWords > 0 {
				wordLen++
			}

			if overlapChars+wordLen > overlap {
				break
			}

			overlapChars += wordLen
			overlapWords++
		}

		// Make sure we always move forward.
		newStart := end - overlapWords

		if newStart <= start {
			newStart = end
		}

		start = newStart
	}
	fmt.Println(" =================== CHUNKING END =================== ")
	return chunks
}

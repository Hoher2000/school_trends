package utils

import (
	"strings"
	"unicode/utf8"
)

func SplitIntoSubtitles(text string) []string {
	// Делим по предложениям
	phrases := SplitByPunctuation(text, []rune{'.', '!', '?'})
	if len(phrases) >= 3 {
		return phrases
	}
	// Если мало предложений — делим по словам
	return SplitByWords(text, 6)
}

func SplitByPunctuation(text string, puncts []rune) []string {
	var result []string
	start := 0
	for i, r := range text {
		for _, p := range puncts {
			if r == p {
				end := i + utf8.RuneLen(r)
				phrase := strings.TrimSpace(text[start:end])
				if phrase != "" {
					result = append(result, phrase)
				}
				start = end
				break
			}
		}
	}
	if start < len(text) {
		phrase := strings.TrimSpace(text[start:])
		if phrase != "" {
			result = append(result, phrase)
		}
	}
	return result
}

func SplitByWords(text string, wordsPerChunk int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var chunks []string
	for i := 0; i < len(words); i += wordsPerChunk {
		end := i + wordsPerChunk
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, strings.Join(words[i:end], " "))
	}
	return chunks
}

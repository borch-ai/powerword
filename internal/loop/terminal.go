package loop

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// TerminalFormatter is a stateful streaming markdown and thinking block parser
// designed to colorize and wrap text in real-time on the CLI.
type TerminalFormatter struct {
	writer  io.Writer
	width   int
	err     error // Stores the first encountered write error
	flushed bool  // Ensures Flush runs only once

	// State variables
	inCodeBlock        bool
	codeBlockLineStart bool
	inThinking         bool
	inHeader           bool
	headerLevel        int
	boldActive         bool
	italicActive       bool

	// Formatting variables
	currentCol    int
	wrapIndent    string
	isLineStart   bool
	pendingSpaces int

	// Buffers
	buf     []byte
	wordBuf []rune
}

// NewTerminalFormatter creates and returns a new TerminalFormatter.
func NewTerminalFormatter(w io.Writer, width int) *TerminalFormatter {
	return &TerminalFormatter{
		writer:      w,
		width:       width,
		isLineStart: true,
	}
}

// writeBytes writes data to the underlying writer, capturing the first error.
func (f *TerminalFormatter) writeBytes(p []byte) {
	if f.err != nil {
		return
	}
	_, err := f.writer.Write(p)
	if err != nil {
		f.err = err
	}
}

// writeString writes a string to the underlying writer, capturing the first error.
func (f *TerminalFormatter) writeString(s string) {
	f.writeBytes([]byte(s))
}

// writeFprintf writes formatted data to the underlying writer, capturing the first error.
func (f *TerminalFormatter) writeFprintf(format string, args ...any) {
	if f.err != nil {
		return
	}
	_, err := fmt.Fprintf(f.writer, format, args...)
	if err != nil {
		f.err = err
	}
}

// Write appends streaming bytes, processes formatting, and outputs colored styled text.
func (f *TerminalFormatter) Write(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.buf = append(f.buf, p...)
	f.process()
	if f.err != nil {
		return 0, f.err
	}
	return len(p), nil
}

// updateStyles writes ANSI codes to synchronize the styling with current flags.
func (f *TerminalFormatter) updateStyles() {
	var parts []string
	parts = append(parts, "0") // Always start with reset

	if f.inThinking {
		parts = append(parts, "3", "90") // Italic, Dim Gray
	}
	if f.inHeader {
		parts = append(parts, "1") // Bold
		switch f.headerLevel {
		case 1:
			parts = append(parts, "35") // Magenta
		case 2:
			parts = append(parts, "34") // Blue
		case 3:
			parts = append(parts, "36") // Cyan
		default:
			parts = append(parts, "32") // Green
		}
	} else if f.inCodeBlock {
		parts = append(parts, "36") // Cyan text for code
	}

	if f.boldActive {
		parts = append(parts, "1")
	}
	if f.italicActive {
		parts = append(parts, "3")
	}

	f.writeString("\x1b[" + strings.Join(parts, ";") + "m")
}

// hasIncompletePrefix checks if the current buffer starts with a prefix of a token
// that is not yet fully readable, meaning we should wait for more bytes.
func (f *TerminalFormatter) hasIncompletePrefix() bool {
	if f.inCodeBlock {
		// Inside code blocks, we only care about the closing fence "```"
		tb := []byte("```")
		return len(f.buf) < len(tb) && bytes.Equal(f.buf, tb[:len(f.buf)])
	}

	// Outside code blocks, check standard targets
	targets := []string{"```", "<think>", "</think>", "**", "*"}
	if f.isLineStart {
		targets = append(targets, "* ", "- ", "+ ")
	}

	for _, t := range targets {
		tb := []byte(t)
		if len(f.buf) < len(tb) && bytes.Equal(f.buf, tb[:len(f.buf)]) {
			return true
		}
	}

	// Check for header prefix (up to 6 hashes at start of line)
	if f.isLineStart && len(f.buf) > 0 && len(f.buf) <= 6 {
		allHash := true
		for _, b := range f.buf {
			if b != '#' {
				allHash = false
				break
			}
		}
		if allHash {
			return true
		}
	}

	// Check for ordered list item prefix (digits followed by dot and optional space)
	if f.isLineStart && len(f.buf) > 0 {
		i := 0
		for i < len(f.buf) && f.buf[i] >= '0' && f.buf[i] <= '9' {
			i++
		}
		if i == len(f.buf) {
			// Buffer consists entirely of digits, might be followed by ". "
			return true
		}
		if i < len(f.buf) && f.buf[i] == '.' && i+1 == len(f.buf) {
			// Buffer consists of "digits.", might be followed by " "
			return true
		}
	}

	return false
}

// flushWord writes the currently buffered word, applying wrapping if necessary.
func (f *TerminalFormatter) flushWord() {
	if len(f.wordBuf) == 0 {
		return
	}

	wordStr := string(f.wordBuf)
	wordVisualLen := stringVisualWidth(f.wordBuf)
	spaceLen := f.pendingSpaces

	if f.width > 0 && f.currentCol+spaceLen+wordVisualLen > f.width && f.currentCol > len(f.wrapIndent) {
		f.writeString("\n")
		f.writeString(f.wrapIndent)
		f.currentCol = len(f.wrapIndent)
		f.pendingSpaces = 0
	} else if f.pendingSpaces > 0 {
		f.writeString(strings.Repeat(" ", f.pendingSpaces))
		f.currentCol += f.pendingSpaces
		f.pendingSpaces = 0
	}

	f.writeString(wordStr)
	f.currentCol += wordVisualLen
	f.wordBuf = f.wordBuf[:0]
}

// appendToWord appends a character to the active word buffer, flushing if it gets too long.
func (f *TerminalFormatter) appendToWord(r rune) {
	f.wordBuf = append(f.wordBuf, r)
	if f.width > 0 {
		limit := f.width - len(f.wrapIndent)
		if limit <= 0 {
			limit = 1
		}
		if stringVisualWidth(f.wordBuf) >= limit {
			f.flushWord()
		}
	}
}

// writeSpace handles printing spaces and tabs, wrapping if necessary.
func (f *TerminalFormatter) writeSpace(r rune) {
	f.flushWord()
	if r == '\t' {
		f.pendingSpaces += 4
	} else {
		f.pendingSpaces += 1
	}
}

// writeNewline handles printing newlines and resets the line state.
func (f *TerminalFormatter) writeNewline() {
	f.flushWord()

	if f.inHeader {
		f.inHeader = false
		f.headerLevel = 0
		f.updateStyles()
	}

	f.writeString("\n")
	f.currentCol = 0
	f.isLineStart = true
	f.pendingSpaces = 0

	if f.inThinking {
		f.wrapIndent = "  "
	} else {
		f.wrapIndent = ""
	}
}

// printCodeBlockHeader prints the top border of a code block sized dynamically.
func (f *TerminalFormatter) printCodeBlockHeader(lang string) {
	w := f.width
	if w <= 0 {
		w = 80
	}
	// Visual box width is w - 4
	dashes := w - len(lang) - 12
	if dashes < 5 {
		dashes = 5
	}
	f.writeFprintf("  \x1b[34m╭── %s ─%s\x1b[0m\n", lang, strings.Repeat("─", dashes))
}

// printCodeBlockFooter prints the bottom border of a code block matching header width.
func (f *TerminalFormatter) printCodeBlockFooter() {
	w := f.width
	if w <= 0 {
		w = 80
	}
	dashes := w - 6
	if dashes < 10 {
		dashes = 10
	}
	if f.isLineStart {
		f.writeFprintf("  \x1b[34m╰%s\x1b[0m\n", strings.Repeat("─", dashes))
	} else {
		f.writeFprintf("\n  \x1b[34m╰%s\x1b[0m\n", strings.Repeat("─", dashes))
	}
}

// consumePlainRune reads a single rune from f.buf, processing it and adjusting column counters.
// Returns false if it needs to wait for more bytes (partial UTF-8).
func (f *TerminalFormatter) consumePlainRune() bool {
	if !utf8.FullRune(f.buf) {
		return false
	}

	r, size := utf8.DecodeRune(f.buf)
	if r == utf8.RuneError && size == 1 {
		// Consume 1 byte as plain text to avoid blocking on genuinely invalid bytes
		f.writeBytes(f.buf[:1])
		f.buf = f.buf[1:]
		f.isLineStart = false
		return true
	}

	runeBytes := f.buf[:size]

	if f.inCodeBlock {
		if f.codeBlockLineStart {
			f.writeString("\x1b[0m  \x1b[34m│\x1b[0m ")
			f.updateStyles()
			f.codeBlockLineStart = false
		}

		if r == '\n' {
			f.writeBytes(runeBytes)
			f.isLineStart = true
			f.codeBlockLineStart = true
		} else {
			f.writeBytes(runeBytes)
			f.isLineStart = false
		}
	} else {
		switch r {
		case '\n':
			f.writeNewline()
		case ' ', '\t':
			f.writeSpace(r)
			f.isLineStart = false
		default:
			f.appendToWord(r)
			f.isLineStart = false
		}
	}

	f.buf = f.buf[size:]
	return true
}

// process scans the internal buffer for formatting tokens and text.
func (f *TerminalFormatter) process() {
	for len(f.buf) > 0 {
		if f.err != nil {
			return
		}

		if f.hasIncompletePrefix() {
			return
		}

		if f.inCodeBlock {
			if bytes.HasPrefix(f.buf, []byte("```")) {
				consumeLen := 3
				if len(f.buf) > 3 && f.buf[3] == '\n' {
					consumeLen = 4
				}

				f.inCodeBlock = false
				f.codeBlockLineStart = false
				f.updateStyles()

				f.printCodeBlockFooter()

				f.buf = f.buf[consumeLen:]
				f.isLineStart = true
				continue
			}

			if !f.consumePlainRune() {
				return
			}
			continue
		}

		// Outside code blocks:
		// Check for opening code block "```"
		if bytes.HasPrefix(f.buf, []byte("```")) {
			idx := bytes.IndexByte(f.buf, '\n')
			consumeLen := 0
			lang := ""
			if idx == -1 {
				if len(f.buf) < 50 {
					return // Wait for newline
				}
				lang = "code"
				consumeLen = 3
			} else {
				lang = strings.TrimSpace(string(f.buf[3:idx]))
				if lang == "" {
					lang = "code"
				}
				consumeLen = idx + 1
			}

			f.flushWord()
			f.inCodeBlock = true
			f.codeBlockLineStart = true

			if !f.isLineStart {
				f.writeString("\n")
			}
			f.printCodeBlockHeader(lang)
			f.updateStyles()

			f.buf = f.buf[consumeLen:]
			f.isLineStart = true
			continue
		}

		// Check for think block "<think>"
		if bytes.HasPrefix(f.buf, []byte("<think>")) {
			f.flushWord()
			f.inThinking = true
			if !f.isLineStart {
				f.writeString("\n")
			}
			f.writeString("  \x1b[1;35m🧠 Thinking...\x1b[0m\n")
			f.wrapIndent = "  "
			f.currentCol = 0
			f.updateStyles()
			f.buf = f.buf[7:]
			f.isLineStart = true
			continue
		}

		if bytes.HasPrefix(f.buf, []byte("</think>")) {
			f.flushWord()
			f.inThinking = false
			f.updateStyles()
			f.writeString("\n")
			f.wrapIndent = ""
			f.currentCol = 0
			f.buf = f.buf[8:]
			f.isLineStart = true
			continue
		}

		// Check list items and headers
		if f.isLineStart {
			// Unordered list
			if len(f.buf) >= 2 && (f.buf[0] == '*' || f.buf[0] == '-' || f.buf[0] == '+') && f.buf[1] == ' ' {
				f.flushWord()
				f.writeString("\x1b[1;35m •\x1b[0m ")
				f.wrapIndent = "   "
				f.currentCol = 3
				f.buf = f.buf[2:]
				f.isLineStart = false
				continue
			}

			// Ordered list
			i := 0
			for i < len(f.buf) && f.buf[i] >= '0' && f.buf[i] <= '9' {
				i++
			}
			if i > 0 && i < len(f.buf) && f.buf[i] == '.' && i+1 < len(f.buf) && f.buf[i+1] == ' ' {
				f.flushWord()
				num := string(f.buf[:i])
				f.writeFprintf("\x1b[1;35m %s.\x1b[0m ", num)
				f.wrapIndent = strings.Repeat(" ", len(num)+3)
				f.currentCol = len(num) + 3
				f.buf = f.buf[i+2:]
				f.isLineStart = false
				continue
			}

			// Headers
			n := 0
			for n < len(f.buf) && f.buf[n] == '#' {
				n++
			}
			if n > 0 && n < len(f.buf) && f.buf[n] == ' ' && n <= 6 {
				f.flushWord()
				f.inHeader = true
				f.headerLevel = n
				switch n {
				case 1:
					f.writeString("\x1b[1;35m█ \x1b[0m")
				case 2:
					f.writeString("\x1b[1;34m▓ \x1b[0m")
				case 3:
					f.writeString("\x1b[1;36m▒ \x1b[0m")
				default:
					f.writeString("\x1b[1;32m░ \x1b[0m")
				}
				f.wrapIndent = "  "
				f.currentCol = 2
				f.updateStyles()
				f.buf = f.buf[n+1:]
				f.isLineStart = false
				continue
			}
		}

		// Bold toggle
		if bytes.HasPrefix(f.buf, []byte("**")) {
			f.flushWord()
			f.boldActive = !f.boldActive
			f.updateStyles()
			f.buf = f.buf[2:]
			continue
		}

		// Italic toggle
		if bytes.HasPrefix(f.buf, []byte("*")) {
			f.flushWord()
			f.italicActive = !f.italicActive
			f.updateStyles()
			f.buf = f.buf[1:]
			continue
		}

		if !f.consumePlainRune() {
			return
		}
	}
}

// Flush outputs any remaining text, closes formatting states, and resets terminal color.
func (f *TerminalFormatter) Flush() error {
	if f.flushed {
		return f.err
	}
	f.flushed = true

	f.flushWord()

	for len(f.buf) > 0 {
		if f.err != nil {
			return f.err
		}

		r, size := utf8.DecodeRune(f.buf)
		if r == utf8.RuneError && size == 1 {
			f.writeBytes(f.buf[:1])
			f.buf = f.buf[1:]
			continue
		}

		if f.inCodeBlock && f.codeBlockLineStart {
			f.writeString("\x1b[0m  \x1b[34m│\x1b[0m ")
			f.updateStyles()
			f.codeBlockLineStart = false
		}

		f.writeBytes(f.buf[:size])
		if r == '\n' {
			f.isLineStart = true
			if f.inCodeBlock {
				f.codeBlockLineStart = true
			}
		} else {
			f.isLineStart = false
		}
		f.buf = f.buf[size:]
	}

	if f.inCodeBlock {
		f.inCodeBlock = false
		f.codeBlockLineStart = false
		f.printCodeBlockFooter()
	}

	f.writeString("\x1b[0m")
	f.boldActive = false
	f.italicActive = false
	f.inThinking = false
	f.inHeader = false
	f.headerLevel = 0
	f.wrapIndent = ""

	return f.err
}

// runeVisualWidth returns the visual column width of a single rune.
func runeVisualWidth(r rune) int {
	if r < 0x20 {
		return 0
	}
	if r == 0x200b || r == 0xfeff || (r >= 0x0300 && r <= 0x036f) {
		return 0
	}
	// Hiragana, Katakana, CJK symbols & punctuation, CJK Unified Ideographs Extension A
	if r >= 0x3000 && r <= 0x4dbf {
		return 2
	}
	// CJK Unified Ideographs, CJK Compatibility Ideographs
	if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) {
		return 2
	}
	// CJK Compatibility Forms, Small Form Variants, Fullwidth forms
	if (r >= 0xfe30 && r <= 0xfe6f) || (r >= 0xff00 && r <= 0xffef) {
		return 2
	}
	// CJK Unified Ideographs Extension B etc. (Supplementary Ideographic Plane)
	if r >= 0x20000 && r <= 0x3ffff {
		return 2
	}
	// Emojis / Pictographs (covering most symbols, emoticons, transport, etc.)
	if (r >= 0x1f000 && r <= 0x1faff) || (r >= 0x2600 && r <= 0x27bf) {
		return 2
	}
	return 1
}

// stringVisualWidth returns the cumulative visual column width of a slice of runes.
func stringVisualWidth(runes []rune) int {
	w := 0
	for _, r := range runes {
		w += runeVisualWidth(r)
	}
	return w
}

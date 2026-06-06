package loop

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func stripANSI(s string) string {
	var buf bytes.Buffer
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if s[i] == 'm' {
				inEscape = false
			}
			continue
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

func TestGetTerminalWidth(t *testing.T) {
	width := getTerminalWidth()
	if width <= 0 {
		t.Errorf("expected terminal width to be > 0, got %d", width)
	}
}

func TestTerminalFormatter_WordWrap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "wrap simple sentence",
			input:    "hello world from powerword loop",
			width:    10,
			expected: "hello\nworld from\npowerword\nloop\x1b[0m",
		},
		{
			name:     "no wrap when width is 0",
			input:    "hello world from powerword loop",
			width:    0,
			expected: "hello world from powerword loop\x1b[0m",
		},
		{
			name:     "split very long word",
			input:    "abcdefghijklmnop",
			width:    5,
			expected: "abcde\nfghij\nklmno\np\x1b[0m",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatter := NewTerminalFormatter(&buf, tc.width)
			_, err := formatter.Write([]byte(tc.input))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			_ = formatter.Flush()

			actual := buf.String()
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestTerminalFormatter_MarkdownFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // Check if these output tokens are present
	}{
		{
			name:  "bold styling",
			input: "this is **bold** text",
			expected: []string{
				"\x1b[0;1m", // Combined bold start style
				"bold",
				"\x1b[0m", // Reset style
			},
		},
		{
			name:  "italic styling",
			input: "this is *italic* text",
			expected: []string{
				"\x1b[0;3m", // Combined italic start style
				"italic",
				"\x1b[0m", // Reset style
			},
		},
		{
			name:  "unordered list item",
			input: "* item one",
			expected: []string{
				"\x1b[1;35m •\x1b[0m ",
				"item",
				"one",
			},
		},
		{
			name:  "ordered list item",
			input: "42. item two",
			expected: []string{
				"\x1b[1;35m 42.\x1b[0m ",
				"item",
				"two",
			},
		},
		{
			name:  "header formatting level 1",
			input: "# Heading 1",
			expected: []string{
				"\x1b[1;35m█ \x1b[0m",
				"\x1b[0;1;35mHeading",
			},
		},
		{
			name:  "header formatting level 2",
			input: "## Heading 2",
			expected: []string{
				"\x1b[1;34m▓ \x1b[0m",
				"\x1b[0;1;34mHeading",
			},
		},
		{
			name:  "header formatting level 3",
			input: "### Heading 3",
			expected: []string{
				"\x1b[1;36m▒ \x1b[0m",
				"\x1b[0;1;36mHeading",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatter := NewTerminalFormatter(&buf, 80)
			_, err := formatter.Write([]byte(tc.input))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			_ = formatter.Flush()

			actual := buf.String()
			for _, exp := range tc.expected {
				if !strings.Contains(actual, exp) {
					t.Errorf("expected output to contain %q, but got %q", exp, actual)
				}
			}
		})
	}
}

func TestTerminalFormatter_CodeBlock(t *testing.T) {
	input := "```go\npackage main\n```"
	var buf bytes.Buffer
	formatter := NewTerminalFormatter(&buf, 80)
	_, err := formatter.Write([]byte(input))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	_ = formatter.Flush()

	actual := buf.String()
	if !strings.Contains(actual, "╭── go ──") {
		t.Errorf("expected header border, got %q", actual)
	}

	plain := stripANSI(actual)
	if !strings.Contains(plain, "│ package main") {
		t.Errorf("expected content line with vertical border, got %q", plain)
	}
	if !strings.Contains(plain, "╰──────") {
		t.Errorf("expected footer border, got %q", plain)
	}
}

func TestTerminalFormatter_ThinkBlock(t *testing.T) {
	input := "<think>\nanalysing problem\n</think>\nactual response"
	var buf bytes.Buffer
	formatter := NewTerminalFormatter(&buf, 80)
	_, err := formatter.Write([]byte(input))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	_ = formatter.Flush()

	actual := buf.String()
	if !strings.Contains(actual, "🧠 Thinking...") {
		t.Errorf("expected thinking header, got %q", actual)
	}
	if !strings.Contains(actual, "analysing") {
		t.Errorf("expected thinking content, got %q", actual)
	}
	if !strings.Contains(actual, "actual response") {
		t.Errorf("expected actual response content, got %q", actual)
	}
}

func TestTerminalFormatter_UTF8RuneSplitting(t *testing.T) {
	// The word is "antigravité", ending with 'é' (UTF-8 bytes: C3 A9).
	// We write it byte-by-byte to verify that rune buffering holds the partial byte correctly.
	var buf bytes.Buffer
	formatter := NewTerminalFormatter(&buf, 80)

	firstPart := "antigravit"
	lastByte1 := byte(0xC3)
	lastByte2 := byte(0xA9)

	_, _ = formatter.Write([]byte(firstPart))
	_, _ = formatter.Write([]byte{lastByte1})
	// Check that we haven't printed the incomplete character yet
	if strings.Contains(buf.String(), "é") {
		t.Errorf("expected not to print 'é' yet, got %q", buf.String())
	}

	_, _ = formatter.Write([]byte{lastByte2})
	_ = formatter.Flush()

	actual := buf.String()
	plain := stripANSI(actual)
	if !strings.Contains(plain, "antigravité") {
		t.Errorf("expected 'antigravité' at the end, got %q", plain)
	}
}

func TestTerminalFormatter_CoverageBonus(t *testing.T) {
	// 1. Test tab character and multiple spaces
	{
		var buf bytes.Buffer
		formatter := NewTerminalFormatter(&buf, 80)
		_, _ = formatter.Write([]byte("hello\tworld"))
		_ = formatter.Flush()
		plain := stripANSI(buf.String())
		if !strings.Contains(plain, "hello    world") {
			t.Errorf("expected tab to print 4 spaces, got %q", plain)
		}
	}

	// 2. Test Flush with active states
	{
		var buf bytes.Buffer
		formatter := NewTerminalFormatter(&buf, 80)
		// Enter thinking and bold
		_, _ = formatter.Write([]byte("<think>**thinking and bold"))
		// Flush while active
		_ = formatter.Flush()
		plain := stripANSI(buf.String())
		if !strings.Contains(plain, "Thinking...") {
			t.Errorf("expected thinking to be flushed, got %q", plain)
		}
		if !strings.Contains(plain, "thinking and bold") {
			t.Errorf("expected content to be flushed, got %q", plain)
		}
	}

	// 3. Test Flush inside a code block
	{
		var buf bytes.Buffer
		formatter := NewTerminalFormatter(&buf, 80)
		_, _ = formatter.Write([]byte("```go\ncode content"))
		_ = formatter.Flush()
		plain := stripANSI(buf.String())
		if !strings.Contains(plain, "code content") {
			t.Errorf("expected code content to be flushed, got %q", plain)
		}
		if !strings.Contains(plain, "╰──────────────────") {
			t.Errorf("expected footer to be printed by Flush, got %q", plain)
		}
	}

	// 4. Test incomplete prefixes
	{
		prefixes := []string{
			"<t",
			"</t",
			"**",
			"*",
			"```",
			"1",
			"12.",
			"###",
		}
		for _, pref := range prefixes {
			var buf bytes.Buffer
			formatter := NewTerminalFormatter(&buf, 80)
			_, _ = formatter.Write([]byte(pref))
			// Should not produce output yet since it's an incomplete prefix
			if buf.Len() > 0 && !strings.Contains(pref, "*") && !strings.Contains(pref, "`") {
				t.Errorf("expected no output for incomplete prefix %q, got %q", pref, buf.String())
			}
			_ = formatter.Flush()
		}
	}

	// 5. Test invalid UTF-8 byte inside Flush
	{
		var buf bytes.Buffer
		formatter := NewTerminalFormatter(&buf, 80)
		formatter.buf = []byte{0x80} // Invalid starting byte
		_ = formatter.Flush()
		if len(buf.Bytes()) == 0 || buf.Bytes()[0] != 0x80 {
			t.Errorf("expected invalid byte 0x80 to be flushed first, got %v", buf.Bytes())
		}
	}

	// 6. Test invalid UTF-8 byte inside consumePlainRune
	{
		var buf bytes.Buffer
		formatter := NewTerminalFormatter(&buf, 80)
		_, _ = formatter.Write([]byte{0x80})
		_ = formatter.Flush()
		plain := stripANSI(buf.String())
		if len(plain) == 0 {
			t.Errorf("expected invalid byte to be written")
		}
	}
}

type errorWriter struct{}

func (e errorWriter) Write(p []byte) (int, error) {
	return 0, errors.New("underlying write error")
}

func TestTerminalFormatter_ErrorPropagation(t *testing.T) {
	formatter := NewTerminalFormatter(errorWriter{}, 80)
	_, err := formatter.Write([]byte("hello world"))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "underlying write error") {
		t.Errorf("expected underlying write error, got %v", err)
	}

	err = formatter.Flush()
	if err == nil {
		t.Fatal("expected flush error, got nil")
	}
	if !strings.Contains(err.Error(), "underlying write error") {
		t.Errorf("expected underlying write error, got %v", err)
	}
}

func TestTerminalFormatter_CJKAndEmojiWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "cjk characters exactly fit width",
			input:    "こんにちは", // 5 Japanese characters, visual width 10
			width:    10,
			expected: "こんにちは\x1b[0m",
		},
		{
			name:     "cjk characters wrap when exceeding width",
			input:    "こんにちは世界", // 7 Japanese characters, visual width 14
			width:    10,
			expected: "こんにちは\n世界\x1b[0m",
		},
		{
			name:     "emoji character width",
			input:    "🧠🧠🧠🧠🧠", // 5 brain emojis, visual width 10
			width:    10,
			expected: "🧠🧠🧠🧠🧠\x1b[0m",
		},
		{
			name:     "emoji characters wrap",
			input:    "🧠🧠🧠🧠🧠🧠", // 6 brain emojis, visual width 12
			width:    10,
			expected: "🧠🧠🧠🧠🧠\n🧠\x1b[0m",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatter := NewTerminalFormatter(&buf, tc.width)
			_, err := formatter.Write([]byte(tc.input))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			_ = formatter.Flush()

			actual := buf.String()
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestTerminalFormatter_EdgeCases(t *testing.T) {
	// 1. Control character (< 0x20), zero-width characters, combining marks, SIP CJK character, and ESC sanitization
	{
		// Test outside code block
		var buf1 bytes.Buffer
		f1 := NewTerminalFormatter(&buf1, 80)
		// \x01 is control char, \u200B is zero width, \u0301 is combining mark, \U00020000 is SIP CJK, \x1b is ESC, \x7f is DEL
		input1 := "\x01a\u200Bb\u0301\U00020000\x1b\x7f"
		_, _ = f1.Write([]byte(input1))
		_ = f1.Flush()
		plain1 := stripANSI(buf1.String())
		if !strings.Contains(plain1, "\uFFFD") {
			t.Errorf("expected control character to be sanitized to replacement character, got %q", plain1)
		}
		if !strings.Contains(plain1, "^[") {
			t.Errorf("expected ESC to be sanitized to '^[', got %q", plain1)
		}

		// Test inside code block
		var buf2 bytes.Buffer
		f2 := NewTerminalFormatter(&buf2, 80)
		input2 := "```go\n\x01\x1b\x7f\n```"
		_, _ = f2.Write([]byte(input2))
		_ = f2.Flush()
		plain2 := stripANSI(buf2.String())
		if !strings.Contains(plain2, "\uFFFD") {
			t.Errorf("expected control characters inside code block to be sanitized, got %q", plain2)
		}
		if !strings.Contains(plain2, "^[") {
			t.Errorf("expected ESC inside code block to be sanitized, got %q", plain2)
		}

		// Test during Flush()
		var buf3 bytes.Buffer
		f3 := NewTerminalFormatter(&buf3, 80)
		_, _ = f3.Write([]byte("some text"))
		// Inject into f.buf directly to force Flush() to process them
		f3.buf = []byte("\x01\x1b\x7f")
		_ = f3.Flush()
		plain3 := stripANSI(buf3.String())
		if !strings.Contains(plain3, "\uFFFD") {
			t.Errorf("expected control characters in Flush to be sanitized, got %q", plain3)
		}
		if !strings.Contains(plain3, "^[") {
			t.Errorf("expected ESC in Flush to be sanitized, got %q", plain3)
		}

		// Test during Flush() inside code block
		var buf4 bytes.Buffer
		f4 := NewTerminalFormatter(&buf4, 80)
		_, _ = f4.Write([]byte("```go\ncode"))
		f4.buf = []byte("\x01\x1b\x7f")
		_ = f4.Flush()
		plain4 := stripANSI(buf4.String())
		if !strings.Contains(plain4, "\uFFFD") {
			t.Errorf("expected control characters in Flush inside code block to be sanitized, got %q", plain4)
		}
		if !strings.Contains(plain4, "^[") {
			t.Errorf("expected ESC in Flush inside code block to be sanitized, got %q", plain4)
		}
	}
}

func TestTerminalFormatter_MoreEdgeCases(t *testing.T) {

	// 2. Code block header and footer with width <= 0 (defaults to 80)
	{
		var buf bytes.Buffer
		formatter := NewTerminalFormatter(&buf, 0)
		_, _ = formatter.Write([]byte("```go\nfmt.Println()\n```"))
		_ = formatter.Flush()
		plain := stripANSI(buf.String())
		if !strings.Contains(plain, "│ fmt.Println()") {
			t.Errorf("expected code block to render even with width 0, got %q", plain)
		}
	}

	// 3. Write after error is set
	{
		formatter := NewTerminalFormatter(errorWriter{}, 80)
		_, _ = formatter.Write([]byte("initial trigger error"))
		// Verify writing again returns error
		_, err := formatter.Write([]byte("more text"))
		if err == nil {
			t.Error("expected error writing when formatter has an active error state")
		}
	}
}

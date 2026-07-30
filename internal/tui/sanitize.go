package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const terminalControlReplacement = '�'

func sanitizeTerminalText(value string, preserveNewlines bool) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")

	// The ANSI parser recognizes the single-byte C1 representation. Convert
	// equivalent Unicode control code points first so C1 CSI/OSC strings are
	// stripped by the same parser as their ESC-prefixed forms.
	var controls strings.Builder
	controls.Grow(len(value))
	for _, character := range value {
		if character >= 0x80 && character <= 0x9f {
			controls.WriteByte(byte(character))
			continue
		}
		controls.WriteRune(character)
	}

	stripped := ansi.Strip(controls.String())
	var sanitized strings.Builder
	sanitized.Grow(len(stripped))
	for _, character := range stripped {
		switch {
		case character == '\n' && preserveNewlines:
			sanitized.WriteByte('\n')
		case character == '\t':
			sanitized.WriteByte(' ')
		case character < 0x20 || character == 0x7f ||
			(character >= 0x80 && character <= 0x9f):
			sanitized.WriteRune(terminalControlReplacement)
		default:
			sanitized.WriteRune(character)
		}
	}
	return sanitized.String()
}

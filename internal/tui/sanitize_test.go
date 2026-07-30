package tui

import "testing"

func TestSanitizeTerminalTextStripsSequencesAndNeutralizesControls(t *testing.T) {
	tests := []struct {
		name             string
		value            string
		preserveNewlines bool
		want             string
	}{
		{
			name:  "ESC and CSI",
			value: "before\x1b[31mred\x1b[0mafter\x1b[2J",
			want:  "beforeredafter",
		},
		{
			name:  "OSC 8 hyperlink",
			value: "before\x1b]8;;https://evil.example\x1b\\label\x1b]8;;\x1b\\after",
			want:  "beforelabelafter",
		},
		{
			name:  "OSC 52 clipboard",
			value: "before\x1b]52;c;c2VjcmV0\x07after",
			want:  "beforeafter",
		},
		{
			name:  "C0 C1 and DEL",
			value: "a\rb\bc\x00d\x7fe\u0085f",
			want:  "a�b�c�d�e�f",
		},
		{
			name:             "body newlines and tabs",
			value:            "first\r\nsecond\nthird\tcolumn",
			preserveNewlines: true,
			want:             "first\nsecond\nthird column",
		},
		{
			name:  "single line newline",
			value: "first\nsecond",
			want:  "first�second",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sanitizeTerminalText(test.value, test.preserveNewlines); got != test.want {
				t.Fatalf("sanitizeTerminalText() = %q, want %q", got, test.want)
			}
		})
	}
}

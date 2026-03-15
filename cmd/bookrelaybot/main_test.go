package main

import "testing"

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "Book - Author", "Book - Author"},
		{"slashes", "A/B\\C", "A_B_C"},
		{"colons", "Title: Subtitle", "Title_ Subtitle"},
		{"quotes", `Say "hello"`, "Say _hello_"},
		{"newlines", "Line1\nLine2\r\nLine3", "Line1_Line2__Line3"},
		{"null byte", "Title\x00Evil", "Title_Evil"},
		{"tabs", "Title\tAuthor", "Title_Author"},
		{"angle brackets", "<script>alert</script>", "_script_alert__script_"},
		{"unicode preserved", "Книга - Автор", "Книга - Автор"},
		{"DEL char", "Title\x7FAuthor", "Title_Author"},
		{"pipe and wildcards", "A|B*C?D", "A_B_C_D"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

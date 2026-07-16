package backup

import "testing"

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":               "Montainer",
		"only punctuation":    "Montainer",
		"safe characters":     "my-server_2.0",
		"unsafe characters":   "my_server",
		"trim unsafe framing": "server",
	}
	inputs := map[string]string{
		"empty":               "  ",
		"only punctuation":    "...---___",
		"safe characters":     "my-server_2.0",
		"unsafe characters":   "my server!",
		"trim unsafe framing": "--server..",
	}

	for name, want := range tests {
		name, input := name, inputs[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeName(input); got != want {
				t.Fatalf("sanitizeName(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

package parser

import "testing"

func TestMgekoUrlToName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "https://www.mgeko.cc/manga/monster-eater/",
			want:  "monster-eater",
		},
		{
			input: "https://www.mgeko.cc/manga/monster-eater/all-chapters/",
			want:  "monster-eater",
		},
		{
			input: "https://www.mgeko.cc/manga/another-manga/",
			want:  "another-manga",
		},
		{
			input: "https://www.mgeko.cc/",
			want:  "",
		},
	}

	for _, tt := range tests {
		got := MgekoUrlToName(tt.input)
		if got != tt.want {
			t.Errorf("MgekoUrlToName(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

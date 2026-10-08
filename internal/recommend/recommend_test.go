package recommend

import "testing"

func TestSubtitlesClash(t *testing.T) {
	cases := []struct {
		sub, title string
		clash      bool
	}{
		{"Secret History", "Mistborn: The Final Empire", true},
		{"", "Mistborn: The Final Empire", false},
		{"The Final Empire", "Mistborn: The Final Empire", false},
		{"A Novel", "A Deadly Education", false},
		{"A Novel", "Upgrade: A Novel", false},
		{"An Arcane History of the Oxford Translators' Revolution", "Babel, or The Necessity of Violence: An Arcane History of the Oxford Translators' Revolution", false},
		{"A Dystopian Novella by the 'British Master of Science Fiction'", "Ogres", false},
	}
	for _, c := range cases {
		if got := subtitlesClash(c.sub, c.title); got != c.clash {
			t.Errorf("subtitlesClash(%q, %q) = %v, want %v", c.sub, c.title, got, c.clash)
		}
	}
}

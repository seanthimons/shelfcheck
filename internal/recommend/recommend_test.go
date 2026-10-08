package recommend

import "testing"

func TestSubtitlesClash(t *testing.T) {
	cases := []struct {
		catTitle, sub, title string
		clash                bool
	}{
		{"Mistborn", "Secret History", "Mistborn: The Final Empire", true},
		{"Mistborn: Secret History", "", "Mistborn: The Final Empire", true},
		{"Mistborn", "", "Mistborn: The Final Empire", false},
		{"Mistborn", "The Final Empire", "Mistborn: The Final Empire", false},
		{"A Deadly Education", "A Novel", "A Deadly Education", false},
		{"Upgrade", "A Novel", "Upgrade: A Novel", false},
		{"Babel, Or, The Necessity of Violence", "An Arcane History of the Oxford Translators' Revolution", "Babel, or The Necessity of Violence: An Arcane History of the Oxford Translators' Revolution", false},
		{"Ogres", "A Dystopian Novella by the 'British Master of Science Fiction'", "Ogres", false},
	}
	for _, c := range cases {
		if got := subtitlesClash(c.catTitle, c.sub, c.title); got != c.clash {
			t.Errorf("subtitlesClash(%q, %q, %q) = %v, want %v", c.catTitle, c.sub, c.title, got, c.clash)
		}
	}
}

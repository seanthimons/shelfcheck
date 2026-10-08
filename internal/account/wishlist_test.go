package account

import "testing"

func TestWishlistMatching(t *testing.T) {
	for in, want := range map[string]string{
		"He Who Fights with Monsters, 1":        "He Who Fights with Monsters",
		"Society of Unknowable Objects A Novel": "Society of Unknowable Objects A Novel",
		"Cadillac Desert: The American West":    "Cadillac Desert",
		"Uprooted":                              "Uprooted",
	} {
		if got := wishQuery(in); got != want {
			t.Errorf("wishQuery(%q) = %q, want %q", in, got, want)
		}
	}
	if AuthorKey("Shirtaloon; Deverell, Travis") != "shirtaloon" || AuthorKey("Tchaikovsky, Adrian") != "tchaikovsky" ||
		AuthorKey("James S. A. Corey") != AuthorKey("Corey, James S. A.") {
		t.Error("authorKey")
	}
	for _, c := range []struct {
		catalog, libby string
		want           bool
	}{
		{"A Discovery of Witches", "Discovery of Witches", true},
		{"The Society of Unknowable Objects", "Society of Unknowable Objects A Novel", true},
		{"Cadillac Desert", "Cadillac Desert The American West and Its Disappearing Water", true},
		{"He Who Fights with Monsters", "He Who Fights with Monsters, 1", true},
		{"Skyward Flight", "Skyward", false},
		{"Before We Forget Kindness", "Before the Coffee Gets Cold", false},
		{"The World of All Souls", "Discovery of Witches", false},
		{"Scythe", "Scythe", true},
		{"Scy", "Scythe", false},
	} {
		if got := SameTitle(c.catalog, c.libby); got != c.want {
			t.Errorf("SameTitle(%q, %q) = %v, want %v", c.catalog, c.libby, got, c.want)
		}
	}
}

func TestSplitSeries(t *testing.T) {
	for in, want := range map[string][2]string{
		"Murderbot diaries, bk. 2":  {"Murderbot diaries", "2"},
		"Murderbot diaries, book 5": {"Murderbot diaries", "5"},
		"Expanse, bk. 9":            {"Expanse", "9"},
		"Daevabad trilogy ; 3":      {"Daevabad trilogy ; 3", ""},
		"Hierarchy":                 {"Hierarchy", ""},
	} {
		if n, o := SplitSeries(in); n != want[0] || o != want[1] {
			t.Errorf("SplitSeries(%q) = %q, %q; want %q", in, n, o, want)
		}
	}
	if WorkKey("The Incandescent: A School Story", "Tesh, Emily") != WorkKey("Incandescent", "Tesh, Emily") {
		t.Error("WorkKey should ignore subtitles and leading articles")
	}
	if WorkKey("He Who Fights with Monsters, 1", "Shirtaloon") != WorkKey("He Who Fights with Monsters", "Shirtaloon; Deverell, Travis") {
		t.Error("WorkKey should ignore series numbers and co-authors")
	}
}

func TestWorkKeyAlternativeTitles(t *testing.T) {
	a := WorkKey("Babel: Or the Necessity of Violence: An Arcane History of the Oxford Translators' Revolution", "Kuang, R. F.")
	b := WorkKey("Babel, or The Necessity of Violence: An Arcane History of the Oxford Translators' Revolution", "R.F. Kuang")
	if a != b || a != "babel|kuang" {
		t.Errorf("Babel keys differ: %q vs %q", a, b)
	}
	if k := WorkKey("Upgrade: A Novel", "Blake Crouch"); k != "upgrade|crouch" {
		t.Errorf("Upgrade key = %q", k)
	}
	if k := WorkKey("Orbital", "Samantha Harvey"); k != "orbital|harvey" {
		t.Errorf("Orbital key = %q", k)
	}
}

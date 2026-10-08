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
	if AuthorKey("Shirtaloon; Deverell, Travis") != "shirtaloon" || AuthorKey("Tchaikovsky, Adrian") != "tchaikovsky" {
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

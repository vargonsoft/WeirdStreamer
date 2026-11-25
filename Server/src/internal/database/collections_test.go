package database

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUserCollections(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	matrix, heat := CollectionTitle{"movie", 603}, CollectionTitle{"movie", 949}
	id, err := db.CreateUserCollection("lib1", "  Favourites ", "", []CollectionTitle{matrix, matrix})
	if err != nil {
		t.Fatal(err)
	}
	c, err := db.UserCollection("lib1", id)
	if err != nil || c.Name != "Favourites" || len(c.Titles) != 1 {
		t.Fatalf("created: %+v %v", c, err)
	}
	if _, err := db.UserCollection("lib2", id); !errors.Is(err, ErrNoUserCollection) {
		t.Errorf("another library's collection: %v", err)
	}

	name, desc := "Crime", "Heists and hackers"
	if err := db.UpdateUserCollection("lib1", id, &name, &desc, []CollectionTitle{heat}, []CollectionTitle{matrix}); err != nil {
		t.Fatal(err)
	}
	c, _ = db.UserCollection("lib1", id)
	if c.Name != "Crime" || c.Description != desc || len(c.Titles) != 1 || c.Titles[0] != heat {
		t.Fatalf("updated: %+v", c)
	}
	if err := db.UpdateUserCollection("lib2", id, nil, nil, nil, nil); !errors.Is(err, ErrNoUserCollection) {
		t.Errorf("updating from another library: %v", err)
	}
	db.CreateUserCollection("lib1", "a first", "", nil)
	list, _ := db.UserCollections("lib1")
	if len(list) != 2 || list[0].Name != "a first" || list[1].Name != "Crime" {
		t.Fatalf("listed: %+v", list)
	}

	if err := db.DeleteUserCollection("lib2", id); !errors.Is(err, ErrNoUserCollection) {
		t.Errorf("deleting from another library: %v", err)
	}
	if err := db.DeleteUserCollection("lib1", id); err != nil {
		t.Fatal(err)
	}
	if titles, _ := db.collectionTitles(id); len(titles) != 0 {
		t.Errorf("its titles stayed: %v", titles)
	}
}

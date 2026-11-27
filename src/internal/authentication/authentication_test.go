package authentication

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Many clients log in, check their tokens and read the users at the same time. Run with -race; without the lock
// Go stops with "concurrent map writes".
func TestConcurrentUse(t *testing.T) {
	// xTeVe creates the empty database before Init
	var dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "authentication.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}
	// As xTeVe does it at start
	if err := SetDefaultUserData(map[string]any{"authentication.web": false}); err != nil {
		t.Fatal(err)
	}
	if err := CreateDefaultUser("admin", "secret"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				token, err := UserAuthentication("admin", "secret")
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := CheckTheValidityOfTheToken(token); err != nil {
					t.Error(err)
					return
				}
				users, err := GetAllUserData()
				if err != nil {
					t.Error(err)
					return
				}
				for range users {
				}
			}
		})
	}
	wg.Wait()

	if _, err := UserAuthentication("admin", "wrong"); err == nil {
		t.Error("a wrong password was accepted")
	}
}

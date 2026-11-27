package authentication

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// bcrypt with its default cost makes the many logins of the tests slow, above all with -race
	bcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}

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

// A database written by older versions keeps working: users log in with their passwords and keep their data.
func TestExistingDatabase(t *testing.T) {
	var dir = t.TempDir()
	var salt = "0123456789abcdefghij"
	var existing = fmt.Sprintf(`{
  "dbVersion": "1.0",
  "hash": "sha256",
  "users": {
    "id-ABC": {
      "_id": "id-ABC",
      "_username": %q,
      "_password": %q,
      "_salt": %q,
      "data": {"authentication.web": true, "authentication.xml": true, "username": "plex", "defaultUser": true}
    }
  }
}`, SHA256("plex", salt), SHA256("secret", salt), salt)
	if err := os.WriteFile(filepath.Join(dir, "authentication.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}

	token, err := UserAuthentication("plex", "secret")
	if err != nil {
		t.Fatal(err)
	}
	id, err := GetUserID(token)
	if err != nil || id != "id-ABC" {
		t.Fatalf("user = %q, err %v", id, err)
	}

	userData, err := ReadUserData(id)
	if err != nil || !userData.Allowed("authentication.web") || userData.Allowed("authentication.api") ||
		userData.Username != "plex" || !userData.DefaultUser {
		t.Errorf("user data = %+v, err %v", userData, err)
	}

	// The web UI gets the data, but no hashes
	users, err := GetAllUserData()
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(users)
	if strings.Contains(string(content), salt) || strings.Contains(string(content), "_password") ||
		!strings.Contains(string(content), `"username":"plex"`) {
		t.Errorf("users for the web UI = %s", content)
	}

	// Saved in the same format
	if err := WriteUserData(id, userData); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "authentication.json"))
	var keys = []string{`"_salt": "` + salt, `"_password"`, `"authentication.xml": true`, `"dbVersion": "1.0"`}
	for _, key := range keys {
		if !strings.Contains(string(saved), key) {
			t.Errorf("saved database lacks %s:\n%s", key, saved)
		}
	}
}

// storedPassword returns the password hash of the only user in the database file.
func storedPassword(t *testing.T, dir string) string {
	t.Helper()
	var content, err = os.ReadFile(filepath.Join(dir, "authentication.json"))
	if err != nil {
		t.Fatal(err)
	}
	var db databaseContent
	if err := json.Unmarshal(content, &db); err != nil {
		t.Fatal(err)
	}
	for _, u := range db.Users {
		return u.Password
	}
	t.Fatal("no user in the database")
	return ""
}

// A password of an older version (SHA256) is replaced with bcrypt at the first successful login.
func TestPasswordRehash(t *testing.T) {
	var dir = t.TempDir()
	var salt = "0123456789abcdefghij"
	var existing = fmt.Sprintf(`{"dbVersion": "1.0", "hash": "sha256", "users": {"id-ABC": {"_id": "id-ABC",
		"_username": %q, "_password": %q, "_salt": %q, "data": {"username": "plex"}}}}`,
		SHA256("plex", salt), SHA256("secret", salt), salt)
	if err := os.WriteFile(filepath.Join(dir, "authentication.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}

	// A wrong password changes nothing
	if _, err := UserAuthentication("plex", "wrong"); err == nil {
		t.Fatal("a wrong password was accepted")
	}
	if got := storedPassword(t, dir); got != SHA256("secret", salt) {
		t.Errorf("password after a failed login = %s, want the old hash", got)
	}

	if _, err := UserAuthentication("plex", "secret"); err != nil {
		t.Fatal(err)
	}
	var hash = storedPassword(t, dir)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret")) != nil {
		t.Errorf("password after the login = %s, want bcrypt", hash)
	}

	// The new hash works, also after reading the file again
	if err := Init(dir+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}
	if _, err := UserAuthentication("plex", "secret"); err != nil {
		t.Errorf("login with the rehashed password: %v", err)
	}
	if _, err := UserAuthentication("plex", "wrong"); err == nil {
		t.Error("a wrong password was accepted after the rehash")
	}
	if _, err := UserAuthentication("nobody", "secret"); err == nil {
		t.Error("an unknown user was accepted")
	}
}

// New and changed passwords are stored with bcrypt; passwords bcrypt would cut are refused.
func TestNewPasswords(t *testing.T) {
	var dir = t.TempDir()
	if err := Init(dir+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}
	if err := CreateDefaultUser("admin", strings.Repeat("x", 73)); err == nil {
		t.Error("a password of 73 bytes was accepted")
	}
	if err := CreateDefaultUser("admin", "secret"); err != nil {
		t.Fatal(err)
	}
	if hash := storedPassword(t, dir); !strings.HasPrefix(hash, "$2") {
		t.Errorf("new password = %s, want bcrypt", hash)
	}

	token, err := UserAuthentication("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	id, err := GetUserID(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := ChangeCredentials(id, "root", "changed"); err != nil {
		t.Fatal(err)
	}
	if err := ChangeCredentials(id, "", strings.Repeat("x", 73)); err == nil {
		t.Error("a changed password of 73 bytes was accepted")
	}
	if _, err := UserAuthentication("root", "changed"); err != nil {
		t.Errorf("login with the changed name and password: %v", err)
	}
	if _, err := UserAuthentication("admin", "secret"); err == nil {
		t.Error("the old name and password still work")
	}
}

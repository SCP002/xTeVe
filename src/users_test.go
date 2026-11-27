package src

import (
	"encoding/json"
	"os"
	"testing"

	"xteve/src/internal/authentication"
)

func userRequest(t *testing.T, v any) RequestStruct {
	t.Helper()
	var data, err = json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return RequestStruct{UserData: data}
}

// The server checks what the web UI sends about users, not only the browser.
func TestSaveUsers(t *testing.T) {
	if err := authentication.Init(t.TempDir()+string(os.PathSeparator), 60); err != nil {
		t.Fatal(err)
	}
	first, err := createFirstUserForAuthentication("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	firstID, _ := authentication.GetUserID(first)

	// New users need a name and a confirmed password, and are never the first user
	for _, form := range []UserForm{
		{Username: "plex", Password: "", Confirm: ""},
		{Username: "plex", Password: "a", Confirm: "b"},
	} {
		if err := saveNewUser(userRequest(t, form)); err == nil {
			t.Errorf("new user %+v was accepted", form)
		}
	}
	var plex = UserForm{Username: "plex", DefaultUser: true, PMS: true,
		Password: "pw", Confirm: "pw"}
	if err := saveNewUser(userRequest(t, plex)); err != nil {
		t.Fatal(err)
	}
	token, err := authentication.UserAuthentication("plex", "pw")
	if err != nil {
		t.Fatal(err)
	}
	plexID, _ := authentication.GetUserID(token)
	if data, _ := authentication.ReadUserData(plexID); data.DefaultUser || !data.PMS {
		t.Errorf("new user data = %+v, want PMS and not the first user", data)
	}

	// The first user keeps the web UI and can not be deleted
	var change = map[string]UserForm{firstID: {UserData: authentication.UserData{Username: "admin", WEB: false}}}
	if err := saveUserData(userRequest(t, change)); err != nil {
		t.Fatal(err)
	}
	if data, _ := authentication.ReadUserData(firstID); !data.WEB || !data.DefaultUser {
		t.Errorf("first user data = %+v, want web access", data)
	}
	if err := saveUserData(userRequest(t, map[string]UserForm{firstID: {Delete: true}})); err == nil {
		t.Error("the first user was deleted")
	}

	// A changed password must be confirmed
	var mismatch = map[string]UserForm{plexID: {UserData: authentication.UserData{Username: "plex"},
		Password: "new", Confirm: "other"}}
	if err := saveUserData(userRequest(t, mismatch)); err == nil {
		t.Error("an unconfirmed password was accepted")
	}
	if err := saveUserData(userRequest(t, map[string]UserForm{plexID: {Delete: true}})); err != nil {
		t.Errorf("deleting another user: %v", err)
	}
}

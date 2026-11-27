package src

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"xteve/src/internal/authentication"
	"xteve/src/internal/config"
)

func activatedSystemAuthentication() error {
	return authentication.Init(System.Folder.Config, 60)
}

func createFirstUserForAuthentication(username, password string) (token string, err error) {
	if err = authentication.CreateDefaultUser(username, password); err != nil {
		return
	}

	if token, err = authentication.UserAuthentication(username, password); err != nil {
		return
	}

	if token, err = authentication.CheckTheValidityOfTheToken(token); err != nil {
		return
	}

	userID, err := authentication.GetUserID(token)
	if err != nil {
		return
	}

	err = authentication.WriteUserData(userID, authentication.UserData{
		Username:    username,
		WEB:         true,
		PMS:         true,
		M3U:         true,
		XML:         true,
		API:         false,
		DefaultUser: true,
	})

	return
}

func tokenAuthentication(token string) (newToken string, err error) {
	if System.ConfigurationWizard.Load() {
		return
	}

	newToken, err = authentication.CheckTheValidityOfTheToken(token)

	return
}

func basicAuth(r *http.Request, level string) (username string, err error) {
	err = errors.New("user authentication failed")

	auth := strings.SplitN(r.Header.Get("Authorization"), " ", 2)

	if len(auth) != 2 || auth[0] != "Basic" {
		return
	}

	payload, err := base64.StdEncoding.DecodeString(auth[1])
	if err != nil {
		return "", err
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return "", errors.New("invalid basic authentication header")
	}

	username = pair[0]
	var password = pair[1]

	token, err := authentication.UserAuthentication(username, password)

	if err != nil {
		return
	}

	err = checkAuthorizationLevel(token, level)

	return
}

func urlAuth(r *http.Request, requestType string) (err error) {
	var level, token string

	var username = r.URL.Query().Get("username")
	var password = r.URL.Query().Get("password")

	switch requestType {
	case "m3u":
		level = "authentication.m3u"
		if config.Get().AuthenticationM3U {
			token, err = authentication.UserAuthentication(username, password)
			if err != nil {
				return
			}
			err = checkAuthorizationLevel(token, level)
		}

	case "xml":
		level = "authentication.xml"
		if config.Get().AuthenticationXML {
			token, err = authentication.UserAuthentication(username, password)
			if err != nil {
				return
			}
			err = checkAuthorizationLevel(token, level)
		}
	}

	return
}

func checkAuthorizationLevel(token, level string) error {
	userID, err := authentication.GetUserID(token)
	if err != nil {
		return err
	}

	userData, err := authentication.ReadUserData(userID)
	if err != nil {
		return err
	}

	if !userData.Allowed(level) {
		return errors.New("no authorization")
	}
	return nil
}

// Package authentication keeps the users of xTeVe (authentication.json) and the login tokens. It is safe for use by
// multiple goroutines.
package authentication

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const tokenLength = 40
const saltLength = 20
const idLength = 10

const databaseFile = "authentication.json"

// UserData are the permissions of a user and the name shown in the web UI.
type UserData struct {
	Username    string `json:"username,omitempty"`
	DefaultUser bool   `json:"defaultUser,omitempty"`
	WEB         bool   `json:"authentication.web"`
	PMS         bool   `json:"authentication.pms"`
	M3U         bool   `json:"authentication.m3u"`
	XML         bool   `json:"authentication.xml"`
	API         bool   `json:"authentication.api"`
}

// Allowed reports whether the user may use a part of xTeVe: "authentication.web", "authentication.pms",
// "authentication.m3u", "authentication.xml" or "authentication.api".
func (d UserData) Allowed(level string) bool {
	switch level {
	case "authentication.web":
		return d.WEB
	case "authentication.pms":
		return d.PMS
	case "authentication.m3u":
		return d.M3U
	case "authentication.xml":
		return d.XML
	case "authentication.api":
		return d.API
	}
	return false
}

// PublicUser is what the web UI gets of a user: no hashes or salts.
type PublicUser struct {
	Data UserData `json:"data"`
}

// user is a user of authentication.json. Name and password are stored as hashes: the name with SHA256 to find the
// user (the web UI keeps it readable in Data anyway), the password with bcrypt. Older versions stored the password
// with SHA256 too; such a password is replaced with bcrypt at the next login, as only then it is known.
type user struct {
	ID       string   `json:"_id"`
	Username string   `json:"_username"`
	Password string   `json:"_password"`
	Salt     string   `json:"_salt"`
	Data     UserData `json:"data"`
}

// bcryptCost is the cost of new password hashes; tests lower it.
var bcryptCost = bcrypt.DefaultCost

// isBcrypt reports whether a stored password is a bcrypt hash ("$2a$10$..."), not one of older versions.
func isBcrypt(hash string) bool { return len(hash) > 0 && hash[0] == '$' }

// hashPassword returns the bcrypt hash of a password. bcrypt uses at most 72 bytes, a longer password is refused
// instead of being cut.
func hashPassword(password string) (string, error) {
	var hash, err = bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return "", errors.New("the password is longer than 72 bytes")
	}
	return string(hash), err
}

// nameMatches reports whether username is the name of u.
func (u *user) nameMatches(username string) bool {
	return subtle.ConstantTimeCompare([]byte(SHA256(username, u.Salt)), []byte(u.Username)) == 1
}

// passwordMatches reports whether password is the password of u.
func (u *user) passwordMatches(password string) bool {
	if isBcrypt(u.Password) {
		return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil
	}
	return subtle.ConstantTimeCompare([]byte(SHA256(password, u.Salt)), []byte(u.Password)) == 1
}

// unknownUser is compared with the password of a name that has no user, so that a wrong name takes as long as a
// wrong password.
var unknownUser = sync.OnceValue(func() *user {
	var hash, _ = bcrypt.GenerateFromPassword([]byte("unknown user"), bcryptCost)
	return &user{Password: string(hash)}
})

type databaseContent struct {
	DBVersion string           `json:"dbVersion"`
	Hash      string           `json:"hash"`
	Users     map[string]*user `json:"users"`
}

type token struct {
	userID  string
	expires time.Time
}

// mu guards the state below: every exported function that uses it holds mu.
var mu sync.Mutex

var (
	database      string
	tokenValidity int
	data          databaseContent
	tokens        = make(map[string]token)
	initialized   = false
)

// Cookie : cookie
type Cookie struct {
	Name       string
	Value      string
	Path       string
	Domain     string
	Expires    time.Time
	RawExpires string
}

// Init loads the users from authentication.json in the folder of databasePath, creating the file if needed. Tokens
// are valid for validity minutes.
func Init(databasePath string, validity int) error {
	mu.Lock()
	defer mu.Unlock()

	database = filepath.Join(filepath.Dir(databasePath), databaseFile)

	var content, err = os.ReadFile(database)
	switch {
	case errors.Is(err, os.ErrNotExist):
		data = databaseContent{}
	case err != nil:
		return err
	default:
		data = databaseContent{}
		if err := json.Unmarshal(content, &data); err != nil {
			return err
		}
	}

	// An empty database (as created at the first start) gets its defaults
	if data.Users == nil {
		data = databaseContent{DBVersion: "1.0", Hash: "sha256", Users: make(map[string]*user)}
		if err := saveDatabase(); err != nil {
			return err
		}
	}

	tokenValidity = validity
	initialized = true
	return nil
}

// CreateDefaultUser creates the first user. It fails if a user exists.
func CreateDefaultUser(username, password string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return err
	}
	if len(data.Users) > 0 {
		return createError(001)
	}

	var u, err = newUser(username, password)
	if err != nil {
		return err
	}
	data.Users[u.ID] = u
	return saveDatabase()
}

// CreateNewUser creates a user and returns its ID. It fails if a user with the name exists.
func CreateNewUser(username, password string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return "", err
	}
	for _, u := range data.Users {
		if u.nameMatches(username) {
			return "", createError(020)
		}
	}

	var u, err = newUser(username, password)
	if err != nil {
		return "", err
	}
	data.Users[u.ID] = u
	return u.ID, saveDatabase()
}

// UserAuthentication logs a user in and returns a new token. The password is checked without holding the lock,
// as bcrypt takes a while and clients log in at the same time.
func UserAuthentication(username, password string) (string, error) {
	mu.Lock()
	if err := checkInit(); err != nil {
		mu.Unlock()
		return "", err
	}
	var id string
	var found = unknownUser()
	for i, u := range data.Users {
		if u.nameMatches(username) {
			id, found = i, &user{Password: u.Password, Salt: u.Salt}
			break
		}
	}
	mu.Unlock()

	// A wrong name is checked against a dummy hash, so that it takes as long as a wrong password
	if !found.passwordMatches(password) || id == "" {
		return "", createError(010)
	}

	// A password of an older version is hashed with bcrypt now that it is known
	var hash string
	if !isBcrypt(found.Password) {
		hash, _ = hashPassword(password)
	}

	mu.Lock()
	defer mu.Unlock()
	var u, ok = data.Users[id]
	if !ok || u.Password != found.Password {
		// Removed or changed meanwhile
		return "", createError(010)
	}
	if hash != "" {
		// If that can not be saved, the old hash in the file still works and is replaced at the next login
		u.Password = hash
		if saveDatabase() != nil {
			u.Password = found.Password
		}
	}
	return setToken(id, "-"), nil
}

// CheckTheValidityOfTheToken checks a token and replaces it with a new one.
func CheckTheValidityOfTheToken(oldToken string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return "", err
	}
	var t, ok = tokens[oldToken]
	if !ok || time.Now().After(t.expires) {
		return "", createError(011)
	}
	return setToken(t.userID, oldToken), nil
}

// GetUserID returns the user of a valid token.
func GetUserID(tok string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return "", err
	}
	var t, ok = tokens[tok]
	if !ok || time.Now().After(t.expires) {
		return "", createError(002)
	}
	return t.userID, nil
}

// WriteUserData saves the data of a user.
func WriteUserData(userID string, userData UserData) error {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return err
	}
	var u, ok = data.Users[userID]
	if !ok {
		return createError(030)
	}
	u.Data = userData
	return saveDatabase()
}

// ReadUserData returns the data of a user.
func ReadUserData(userID string) (UserData, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return UserData{}, err
	}
	var u, ok = data.Users[userID]
	if !ok {
		return UserData{}, createError(031)
	}
	return u.Data, nil
}

// RemoveUser removes a user.
func RemoveUser(userID string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return err
	}
	if _, ok := data.Users[userID]; !ok {
		return createError(032)
	}
	delete(data.Users, userID)
	return saveDatabase()
}

// ChangeCredentials changes the name and, if not empty, the password of a user.
func ChangeCredentials(userID, username, password string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return err
	}
	var u, ok = data.Users[userID]
	if !ok {
		return createError(032)
	}
	if len(password) > 0 {
		var hash, err = hashPassword(password)
		if err != nil {
			return err
		}
		u.Password = hash
	}
	if len(username) > 0 {
		u.Username = SHA256(username, u.Salt)
	}
	return saveDatabase()
}

// GetAllUserData returns the users by ID, without their hashes.
func GetAllUserData() (map[string]PublicUser, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkInit(); err != nil {
		return nil, err
	}
	var users = make(map[string]PublicUser, len(data.Users))
	for id, u := range data.Users {
		users[id] = PublicUser{Data: u.Data}
	}
	return users, nil
}

// CheckTheValidityOfTheTokenFromHTTPHeader : get token from HTTP header
func CheckTheValidityOfTheTokenFromHTTPHeader(w http.ResponseWriter, r *http.Request) (newToken string, err error) {
	err = createError(011)
	for _, cookie := range r.Cookies() {
		if cookie.Name == "Token" {
			var token string
			token, err = CheckTheValidityOfTheToken(cookie.Value)
			SetCookieToken(w, token)
			newToken = token
		}
	}
	return
}

// SetCookieToken : set cookie
func SetCookieToken(w http.ResponseWriter, token string) {
	expiration := time.Now().Add(time.Minute * time.Duration(tokenValidity))
	cookie := http.Cookie{Name: "Token", Value: token, Expires: expiration}
	http.SetCookie(w, &cookie)
}

func checkInit() error {
	if !initialized {
		return createError(000)
	}
	return nil
}

// saveDatabase writes the database. mu must be held.
func saveDatabase() error {
	var content, err = json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(database, content, 0600)
}

func newUser(username, password string) (*user, error) {
	var hash, err = hashPassword(password)
	if err != nil {
		return nil, err
	}
	var salt = randomString(saltLength)
	return &user{
		ID:       "id-" + randomID(idLength),
		Username: SHA256(username, salt),
		Password: hash,
		Salt:     salt,
	}, nil
}

// setToken replaces oldToken with a new token of the user. mu must be held.
func setToken(userID, oldToken string) string {
	delete(tokens, oldToken)

	for {
		var newToken = randomString(tokenLength)
		if _, ok := tokens[newToken]; !ok {
			tokens[newToken] = token{
				userID:  userID,
				expires: time.Now().Add(time.Minute * time.Duration(tokenValidity)),
			}
			return newToken
		}
	}
}

// SHA256 is the hash of names, and of passwords of older versions: an HMAC with the secret as key over a fixed text.
// The salt is not used, which older versions did the same way.
func SHA256(secret, salt string) string {
	key := []byte(secret)
	h := hmac.New(sha256.New, key)
	h.Write([]byte("_remote_db"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func randomString(n int) string {
	const alphanum = "-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789aBcDeFgHiJkLmNoPqRsTuVwXyZ_"

	var bytes = make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

func randomID(n int) string {
	const alphanum = "ABCDEFGHJKLMNOPQRSTUVWXYZ0123456789"

	var bytes = make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

func createError(errCode int) (err error) {
	var errMsg string
	switch errCode {
	case 000:
		errMsg = "Authentication has not yet been initialized"
	case 001:
		errMsg = "Default user already exists"
	case 002:
		errMsg = "No user id found for this token"
	case 010:
		errMsg = "User authentication failed"
	case 011:
		errMsg = "Session has expired"
	case 020:
		errMsg = "User already exists"
	case 030:
		errMsg = "User data could not be saved"
	case 031:
		errMsg = "User data could not be read"
	case 032:
		errMsg = "User ID was not found"
	}

	err = errors.New(errMsg)
	return
}

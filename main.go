package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	config = oauth2.Config{
		ClientID:    "test_cli_app",
		RedirectURL: "http://localhost:8080/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost:8088/realms/test1_realm/protocol/openid-connect/auth",
			TokenURL: "http://localhost:8088/realms/test1_realm/protocol/openid-connect/token",
		},
		Scopes: []string{"openid", "profile", "email"},
	}
	state        string
	codeVerifier string
	server       *http.Server
	accessToken  string

	authSuccess = make(chan bool)
)

func main() {

	// Prepare auth request
	state = generateRandomString(32)
	codeVerifier = generateRandomString(32)
	url := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("code_challenge_method", "S256"), oauth2.SetAuthURLParam("code_challenge", sha256Of(codeVerifier)))

	// Run local http server to handle Keycloak callback
	server = &http.Server{Addr: ":8080"}
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/success", successHandler)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Open browser to make request and open Keycloak login form
	cmd := exec.Command("/usr/bin/xdg-open", url)
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to open browser: %v", err)
	}

	// We are waiting for access token
	<-authSuccess

	// Start to process CLI application main loop
	processInput()
}

func generateRandomString(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Error generating random string: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func sha256Of(s string) string {
	b := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {

	// Checking that Keycloak returned the correct OIDC state
	if r.URL.Query().Get("state") != state {
		http.Error(w, "State mismatch", http.StatusBadRequest)
		return
	}

	// Checking that Keycloak returned the Authorization Code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code not found", http.StatusBadRequest)
		return
	}

	// Trying to obtain tokens from Keycloak using PKCE code verifier
	token, err := config.Exchange(context.Background(), code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}
	accessToken = token.AccessToken

	// Redirecting user to success auth page
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<html><head><meta http-equiv="refresh" content="0; url=/success" /></head></html>`)

	// Sending auth success signal, we can start main app logic now
	authSuccess <- true
}

func successHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `<html><body><h1>Authentication successful</h1><p>You can close this window now.</p></body></html>`)

	// Shutting http server down if auth was success
	go func() {
		time.Sleep(5 * time.Second)
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down the server: %s", err)
		}
	}()
}

func processInput() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("\nAllowed commands: ping, token, exit")

	fmt.Print("Please, enter your command: ")
	for scanner.Scan() {
		input := strings.ToLower(scanner.Text())
		if input == "exit" {
			fmt.Println("Exiting...")
			break
		} else if input == "token" {
			fmt.Println("Your access token: " + accessToken)
		} else if input == "ping" {
			fmt.Println("pong")
		}

		// Any real commands can be here, including calling remote API using obtained access token

		fmt.Print("\nPlease, enter your command: ")
	}
}

package backend

import (
	"fmt"
	"net/http"
	"net/url"
)

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	fmt.Println("Got code:", code)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	token, err := exchangeForToken(data)
	if err != nil {
		fmt.Println("Error:", err)
		fmt.Fprintln(w, "Something went wrong, check your terminal.")
		return
	}

	saveToken(token)
	fmt.Println("Access token:", token.AccessToken)
	fmt.Fprintln(w, "Logged in! You can close this tab and go back to the app.")

	// Hand the token back to GetAccessTokenFull, which is waiting on the
	// other end of this channel.
	tokenChan <- token
}

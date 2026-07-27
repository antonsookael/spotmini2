package playback

import (
	"fmt"
	"io"
	"net/http"
)

func ToggleShuffle(accessToken string, state bool) error {
	url := fmt.Sprintf("https://api.spotify.com/v1/me/player/shuffle?state=%t", state)

	req, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Println("Status:", resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Response body:", string(body))

	return nil
}

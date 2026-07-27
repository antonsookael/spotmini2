package playback

import (
	"fmt"
	"io"
	"net/http"
)

func PausePlayback(accessToken string) error {
	req, err := http.NewRequest("PUT", "https://api.spotify.com/v1/me/player/pause", nil)
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

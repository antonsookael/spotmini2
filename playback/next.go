package playback

import (
	"net/http"
)

func NextTrack(accessToken string) error {
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.spotify.com/v1/me/player/next", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

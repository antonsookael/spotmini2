package playback

import "fmt"

func ToggleShuffle(accessToken string, state bool) error {
	url := fmt.Sprintf("https://api.spotify.com/v1/me/player/shuffle?state=%t", state)
	return doPlayerRequest("PUT", url, accessToken, nil)
}

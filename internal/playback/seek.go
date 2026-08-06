package playback

import "fmt"

// Seek jumps to positionMs within the currently playing track.
func Seek(accessToken string, positionMs int) error {
	url := fmt.Sprintf("https://api.spotify.com/v1/me/player/seek?position_ms=%d", positionMs)
	return doPlayerRequest("PUT", url, accessToken, nil)
}

package playback

func PausePlayback(accessToken string) error {
	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/player/pause", accessToken, nil)
}

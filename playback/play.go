package playback

func PlayPlayback(accessToken string) error {
	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/player/play", accessToken, nil)
}

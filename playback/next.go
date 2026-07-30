package playback

func NextTrack(accessToken string) error {
	return doPlayerRequest("POST", "https://api.spotify.com/v1/me/player/next", accessToken, nil)
}

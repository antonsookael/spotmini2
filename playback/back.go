package playback

func PreviousTrack(accessToken string) error {
	return doPlayerRequest("POST", "https://api.spotify.com/v1/me/player/previous", accessToken, nil)
}

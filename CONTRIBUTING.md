# Contributing / maintenance notes

## Reporting a Windows Defender false positive

Unsigned Go binaries get flagged more than average, and this one does a
few things that look "suspicious" to heuristics even though they're
normal for what it is (opens a local-only HTTP listener on
`127.0.0.1:8888` for the OAuth PKCE callback, registers global hotkeys
via `RegisterHotKey`, makes outbound HTTPS calls to
`accounts.spotify.com`/`api.spotify.com`). If a release gets flagged:

1. On a machine where it was flagged, open **Windows Security -> Virus
   & threat protection -> Protection history**, find the detection, and
   note its exact name (e.g. `Trojan:Win32/Wacatac.B!ml`,
   `PUA:Win32/...`). The submission form asks for this.
2. Go to the [Microsoft Security Intelligence submission
   form](https://www.microsoft.com/en-us/wdsi/filesubmission).
3. Sign in with a Microsoft account if you want to track the
   submission's status later (not required to submit).
4. Choose **Software developer** as the submitter type - this is
   specifically for reporting your own software as a false positive,
   and gets different handling than a random "I think this file is
   malware" report.
5. Upload the flagged `.exe` (the same one from the GitHub Release, not
   a rebuilt copy - the detection is tied to that exact binary's hash).
6. Set the submission type to "Incorrectly detected as malware/
   unwanted software" (exact wording may vary), enter the detection
   name from step 1, and in the comments explain what the app is: an
   open-source Spotify now-playing widget, link the GitHub repo, and
   briefly describe the network/hotkey behavior above so the reviewer
   knows it's expected.
7. Submit, and note the submission ID. Check status later at the
   [submission history page](https://www.microsoft.com/en-us/wdsi/submissionhistory).

Turnaround is typically 24-72 hours but can run longer. Approval fixes
Defender's detection for everyone downloading that release, not just
the person who submitted it - but a *new* release is a different binary
hash, so this may need repeating per release until the project builds
enough reputation (or gets code-signed) that it stops happening.

package github

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// ErrNoToken is returned when neither GITHUB_TOKEN nor `gh auth token` yields a credential.
var ErrNoToken = errors.New("no GitHub token: set GITHUB_TOKEN or run `gh auth login`")

// Token returns a GitHub API token from GITHUB_TOKEN, GH_TOKEN, or the gh CLI keyring.
func Token() (string, error) {
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(env)); t != "" {
			return t, nil
		}
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", ErrNoToken
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", ErrNoToken
	}
	t := strings.TrimSpace(string(out))
	if t == "" {
		return "", ErrNoToken
	}
	return t, nil
}

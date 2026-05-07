package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

// SaveToken atomically writes the token to path with mode 0600 (Unix). On
// Windows, Chmod is a silent no-op — token security relies on the parent
// directory's ACLs (a known limitation).
func SaveToken(path string, tok *oauth2.Token) error {
	if tok == nil {
		return errors.New("nil token")
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Tighten in case the dir already existed permissively (MkdirAll only sets
	// perms on dirs it actually creates).
	_ = os.Chmod(dir, 0o700)
	tmp, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func LoadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// User is the merged response from /users/me + /users/me/profile.
type User struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// Me fetches the current user's id+team_id from /users/me. Display name
// requires a separate call to /users/me/profile (see MeWithProfile).
func (c *Client) Me(ctx context.Context) (*User, error) {
	resp, err := c.doCtx(ctx, http.MethodGet, "/users/me", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env struct {
		TeamUser struct {
			UserID string `json:"user_id"`
			TeamID string `json:"team_id"`
		} `json:"team_user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &User{ID: env.TeamUser.UserID, TeamID: env.TeamUser.TeamID}, nil
}

// MeWithProfile fetches /users/me and /users/me/profile and merges them.
// Used by `canva whoami`.
func (c *Client) MeWithProfile(ctx context.Context) (*User, error) {
	u, err := c.Me(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.doCtx(ctx, http.MethodGet, "/users/me/profile", nil)
	if err != nil {
		return u, nil // graceful degradation — id+team_id still useful
	}
	defer resp.Body.Close()
	var env struct {
		Profile struct {
			DisplayName string `json:"display_name"`
		} `json:"profile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return u, nil
	}
	u.DisplayName = env.Profile.DisplayName
	return u, nil
}

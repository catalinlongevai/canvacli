package api

import (
	"context"
	"encoding/json"
	"net/http"
)

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
}

func (c *Client) Me(ctx context.Context) (*User, error) {
	resp, err := c.doCtx(ctx, http.MethodGet, "/users/me", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env struct {
		Team struct {
			User User `json:"user"`
		} `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Team.User, nil
}

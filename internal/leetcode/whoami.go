package leetcode

import (
	"context"
	"errors"
	"fmt"
)

const qUserStatus = `
query globalData {
  userStatus {
    isSignedIn
    username
  }
}`

type userStatusResp struct {
	UserStatus struct {
		IsSignedIn bool   `json:"isSignedIn"`
		Username   string `json:"username"`
	} `json:"userStatus"`
}

// WhoAmI returns the signed-in username, or "" if the request is anonymous or
// the session has expired.
func (c *Client) WhoAmI(ctx context.Context) (string, error) {
	var resp userStatusResp
	if err := c.graphql(ctx, "globalData", qUserStatus, nil, &resp); err != nil {
		return "", err
	}
	if !resp.UserStatus.IsSignedIn {
		return "", nil
	}
	return resp.UserStatus.Username, nil
}

// ErrSessionExpired is returned by VerifySession when the client holds
// credentials that LeetCode no longer recognizes (e.g. an expired cookie).
var ErrSessionExpired = errors.New("session expired")

// VerifySession confirms a client with stored credentials still has a live
// session. Call sites that trust an authenticated-only response (cached solve
// status, Run/Submit) should check this first: an expired cookie otherwise
// makes those calls fall back to an anonymous/empty result instead of
// erroring, which looks like silently lost progress rather than a stale
// login.
func (c *Client) VerifySession(ctx context.Context) error {
	user, err := c.WhoAmI(ctx)
	if err != nil {
		return fmt.Errorf("could not verify session: %w", err)
	}
	if user == "" {
		return ErrSessionExpired
	}
	return nil
}

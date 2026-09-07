package leetcode

import "context"

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

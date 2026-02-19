// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

const (
	UserAuthServiceTaruvi = "taruvi"
)

type TaruviAuthRequest struct {
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
}

type TaruviAuthResponse struct {
	Status int `json:"status"`
	Data   struct {
		User struct {
			ID       int    `json:"id"`
			Display  string `json:"display"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"data"`
	Meta struct {
		IsAuthenticated bool   `json:"is_authenticated"`
		SessionToken    string `json:"session_token"`
		AccessToken     string `json:"access_token"`
		RefreshToken    string `json:"refresh_token"`
		ExpiresIn       int    `json:"expires_in"`
		TokenType       string `json:"token_type"`
	} `json:"meta"`
}

type TaruviUserResponse struct {
	ID        int    `json:"id"`
	UUID      string `json:"uuid"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	IsActive  bool   `json:"is_active"`
}

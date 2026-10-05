// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
)

type KeycloakInterface interface {
	AuthenticateUser(rctx request.CTX, username, password string) (*model.KeycloakUserInfo, *model.AppError)
}

type KeycloakProvider struct {
	app *App
}

func (a *App) Keycloak() KeycloakInterface {
	return &KeycloakProvider{app: a}
}

func (kp *KeycloakProvider) AuthenticateUser(rctx request.CTX, username, password string) (*model.KeycloakUserInfo, *model.AppError) {
	config := kp.app.Config()
	serverURL := *config.KeycloakSettings.ServerURL
	realm := *config.KeycloakSettings.Realm
	clientID := *config.KeycloakSettings.ClientID
	clientSecret := *config.KeycloakSettings.ClientSecret
	timeout := time.Duration(*config.KeycloakSettings.ConnectionTimeout) * time.Second

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", serverURL, realm)

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", clientID)
	form.Set("username", username)
	form.Set("password", password)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	rctx.Logger().Info("Keycloak authentication request",
		mlog.String("token_url", tokenURL),
		mlog.String("username", username))

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, model.NewAppError("KeycloakProvider.AuthenticateUser", "api.keycloak.authenticate.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		rctx.Logger().Error("Failed to connect to Keycloak", mlog.Err(err))
		return nil, model.NewAppError("KeycloakProvider.AuthenticateUser", "api.keycloak.authenticate.connection_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.NewAppError("KeycloakProvider.AuthenticateUser", "api.keycloak.authenticate.read_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	if resp.StatusCode != http.StatusOK {
		var kcErr model.KeycloakErrorResponse
		json.Unmarshal(body, &kcErr)
		rctx.Logger().Error("Keycloak authentication failed",
			mlog.Int("status_code", resp.StatusCode),
			mlog.String("error", kcErr.Error),
			mlog.String("error_description", kcErr.ErrorDescription))
		return nil, model.NewAppError("KeycloakProvider.AuthenticateUser", "api.keycloak.authenticate.invalid_credentials", nil, fmt.Sprintf("Keycloak: %s", kcErr.ErrorDescription), http.StatusUnauthorized)
	}

	rctx.Logger().Info("Keycloak authentication successful", mlog.String("username", username))

	// Token endpoint returned 200 — credentials are valid.
	// No need to call userinfo; MM user is looked up by loginId separately.
	return &model.KeycloakUserInfo{Email: username}, nil
}

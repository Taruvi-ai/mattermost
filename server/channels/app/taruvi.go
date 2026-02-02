// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
)

type TaruviInterface interface {
	AuthenticateUser(rctx request.CTX, username, password string) (*model.TaruviAuthResponse, *model.AppError)
	GetUserInfo(rctx request.CTX, accessToken string) (*model.TaruviUserResponse, *model.AppError)
}

type TaruviProvider struct {
	app *App
}

func (a *App) Taruvi() TaruviInterface {
	return &TaruviProvider{app: a}
}

func (tp *TaruviProvider) AuthenticateUser(rctx request.CTX, username, password string) (*model.TaruviAuthResponse, *model.AppError) {
	config := tp.app.Config()

	serverURL := *config.TaruviSettings.TaruviServerURL
	authEndpoint := *config.TaruviSettings.AuthEndpoint
	timeout := time.Duration(*config.TaruviSettings.ConnectionTimeout) * time.Second

	url := serverURL + authEndpoint

	reqBody := model.TaruviAuthRequest{
		Email:    username,
		Password: password,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.marshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Only set Host header if explicitly configured in settings
	if config.TaruviSettings.OverrideHost != nil && *config.TaruviSettings.OverrideHost {
		hostValue := *config.TaruviSettings.HostOverrideValue
		if hostValue != "" {
			req.Host = hostValue
		}
	}

	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		rctx.Logger().Error("Failed to connect to Taruvi server", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.connection_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.read_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.invalid_credentials", nil, fmt.Sprintf("Status: %d", resp.StatusCode), http.StatusUnauthorized)
	}

	var authResp model.TaruviAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		rctx.Logger().Error("Failed to parse Taruvi response", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.unmarshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	return &authResp, nil
}

func (tp *TaruviProvider) GetUserInfo(rctx request.CTX, accessToken string) (*model.TaruviUserResponse, *model.AppError) {
	config := tp.app.Config()

	serverURL := *config.TaruviSettings.TaruviServerURL
	userEndpoint := *config.TaruviSettings.UserEndpoint
	timeout := time.Duration(*config.TaruviSettings.ConnectionTimeout) * time.Second

	url := serverURL + userEndpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.GetUserInfo", "api.taruvi.get_user.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		rctx.Logger().Error("Failed to connect to Taruvi server", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.GetUserInfo", "api.taruvi.get_user.connection_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.GetUserInfo", "api.taruvi.get_user.read_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, model.NewAppError("TaruviProvider.GetUserInfo", "api.taruvi.get_user.request_failed", nil, fmt.Sprintf("Status: %d", resp.StatusCode), http.StatusUnauthorized)
	}

	var userResp model.TaruviUserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		rctx.Logger().Error("Failed to parse Taruvi user response", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.GetUserInfo", "api.taruvi.get_user.unmarshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	return &userResp, nil
}

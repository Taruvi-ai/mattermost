// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	rctx.Logger().Info("Taruvi authentication request",
		mlog.String("url", url),
		mlog.String("server_url", serverURL),
		mlog.String("auth_endpoint", authEndpoint),
		mlog.String("login_id", username))

	emailCandidate := strings.ToLower(username)
	isEmail := model.IsValidEmail(emailCandidate)

	reqBody := model.TaruviAuthRequest{
		Password: password,
	}
	if isEmail {
		reqBody.Email = emailCandidate
	} else {
		reqBody.Username = username
	}

	if isEmail {
		rctx.Logger().Info("Taruvi auth params",
			mlog.String("email", reqBody.Email))
	} else {
		rctx.Logger().Info("Taruvi auth params",
			mlog.String("username", reqBody.Username))
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.marshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	rctx.Logger().Debug("Taruvi request body", mlog.String("body", string(jsonData)))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.AuthenticateUser", "api.taruvi.authenticate.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set Host header if configured
	if config.TaruviSettings.OverrideHost != nil && *config.TaruviSettings.OverrideHost {
		hostValue := *config.TaruviSettings.HostOverrideValue
		if hostValue != "" {
			req.Host = hostValue
			rctx.Logger().Info("Taruvi Host header set", mlog.String("host", hostValue))
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

	rctx.Logger().Info("Taruvi response", 
		mlog.Int("status_code", resp.StatusCode),
		mlog.String("response_body", string(body)))

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

	// Set Host header if configured
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

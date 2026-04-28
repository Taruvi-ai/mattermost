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
	AuthenticateUser(rctx request.CTX, username, password, authType string) (*model.TaruviAuthResponse, *model.AppError)
}

type TaruviProvider struct {
	app *App
}

func (a *App) Taruvi() TaruviInterface {
	return &TaruviProvider{app: a}
}

func (tp *TaruviProvider) AuthenticateUser(rctx request.CTX, username, password, authType string) (*model.TaruviAuthResponse, *model.AppError) {
	if authType == "session" {
		rctx.Logger().Info("Using session token authentication")
		return tp.validateSessionToken(rctx, password)
	}

	// Check if password is a JWT token (starts with "eyJ")
	rctx.Logger().Info("Checking password format", 
		mlog.Int("password_length", len(password)),
		mlog.String("password_prefix", getPrefix(password, 10)))
	
	if isJWTToken(password) {
		rctx.Logger().Info("Detected JWT token, using token verification")
		return tp.verifyJWTToken(rctx, password)
	}

	// Normal password authentication
	rctx.Logger().Info("Using password authentication")
	return tp.authenticateWithPassword(rctx, username, password)
}

// getPrefix safely gets the first n characters of a string
func getPrefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// isJWTToken checks if the string is a JWT token
func isJWTToken(s string) bool {
	return len(s) > 10 && (s[:3] == "eyJ" || s[:4] == "eyJ0")
}

// verifyJWTToken verifies a JWT token with Taruvi
func (tp *TaruviProvider) verifyJWTToken(rctx request.CTX, token string) (*model.TaruviAuthResponse, *model.AppError) {
	rctx.Logger().Info("verifyJWTToken called", mlog.String("token_prefix", getPrefix(token, 20)))
	
	config := tp.app.Config()
	serverURL := *config.TaruviSettings.TaruviServerURL
	verifyEndpoint := *config.TaruviSettings.VerifyEndpoint
	timeout := time.Duration(*config.TaruviSettings.ConnectionTimeout) * time.Second

	url := serverURL + verifyEndpoint

	rctx.Logger().Info("Taruvi JWT verification request", mlog.String("url", url))

	reqBody := map[string]string{"token": token}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.verifyJWTToken", "api.taruvi.verify.marshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.verifyJWTToken", "api.taruvi.verify.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req.Header.Set("Content-Type", "application/json")

	if config.TaruviSettings.OverrideHost != nil && *config.TaruviSettings.OverrideHost {
		hostValue := *config.TaruviSettings.HostOverrideValue
		if hostValue != "" {
			req.Host = hostValue
			rctx.Logger().Info("Setting Host header for verify", mlog.String("host", hostValue))
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		rctx.Logger().Error("Failed to verify JWT with Taruvi", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.verifyJWTToken", "api.taruvi.verify.connection_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.verifyJWTToken", "api.taruvi.verify.read_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	rctx.Logger().Info("Taruvi verify response", 
		mlog.Int("status_code", resp.StatusCode),
		mlog.String("response_body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return nil, model.NewAppError("TaruviProvider.verifyJWTToken", "api.taruvi.verify.invalid_token", nil, fmt.Sprintf("Status: %d, Body: %s", resp.StatusCode, string(body)), http.StatusUnauthorized)
	}

	// JWT is valid, now get user info using the token
	return tp.getUserInfoWithToken(rctx, token)
}

// authenticateWithPassword performs normal password authentication
func (tp *TaruviProvider) authenticateWithPassword(rctx request.CTX, username, password string) (*model.TaruviAuthResponse, *model.AppError) {
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

// getUserInfoWithToken gets user info using a valid JWT token
func (tp *TaruviProvider) getUserInfoWithToken(rctx request.CTX, token string) (*model.TaruviAuthResponse, *model.AppError) {
	config := tp.app.Config()
	serverURL := *config.TaruviSettings.TaruviServerURL
	userEndpoint := *config.TaruviSettings.UserEndpoint
	timeout := time.Duration(*config.TaruviSettings.ConnectionTimeout) * time.Second

	url := serverURL + userEndpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.getUserInfoWithToken", "api.taruvi.get_user.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	if config.TaruviSettings.OverrideHost != nil && *config.TaruviSettings.OverrideHost {
		hostValue := *config.TaruviSettings.HostOverrideValue
		if hostValue != "" {
			req.Host = hostValue
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		rctx.Logger().Error("Failed to get user info from Taruvi", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.getUserInfoWithToken", "api.taruvi.get_user.connection_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.getUserInfoWithToken", "api.taruvi.get_user.read_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	rctx.Logger().Info("Taruvi get user response", 
		mlog.Int("status_code", resp.StatusCode),
		mlog.String("response_body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return nil, model.NewAppError("TaruviProvider.getUserInfoWithToken", "api.taruvi.get_user.request_failed", nil, fmt.Sprintf("Status: %d, Body: %s", resp.StatusCode, string(body)), http.StatusUnauthorized)
	}

	var rawResp struct {
		Data model.TaruviUserResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &rawResp); err != nil {
		rctx.Logger().Error("Failed to parse Taruvi user response", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.getUserInfoWithToken", "api.taruvi.get_user.unmarshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	userResp := rawResp.Data

	// Convert TaruviUserResponse to TaruviAuthResponse format
	authResp := &model.TaruviAuthResponse{}
	authResp.Data.User.ID = userResp.ID
	authResp.Data.User.Email = userResp.Email
	authResp.Data.User.Username = userResp.Username
	authResp.Data.User.Display = userResp.FirstName + " " + userResp.LastName

	return authResp, nil
}

// validateSessionToken validates a Taruvi allauth session token by calling the session endpoint.
func (tp *TaruviProvider) validateSessionToken(rctx request.CTX, sessionToken string) (*model.TaruviAuthResponse, *model.AppError) {
	config := tp.app.Config()
	serverURL := *config.TaruviSettings.TaruviServerURL
	sessionEndpoint := *config.TaruviSettings.SessionEndpoint
	timeout := time.Duration(*config.TaruviSettings.ConnectionTimeout) * time.Second

	url := serverURL + sessionEndpoint

	rctx.Logger().Info("Taruvi session token validation request", mlog.String("url", url))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.ValidateSessionToken", "api.taruvi.session.request_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	req.Header.Set("X-Session-Token", sessionToken)
	req.Header.Set("Content-Type", "application/json")

	if config.TaruviSettings.OverrideHost != nil && *config.TaruviSettings.OverrideHost {
		hostValue := *config.TaruviSettings.HostOverrideValue
		if hostValue != "" {
			req.Host = hostValue
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		rctx.Logger().Error("Failed to validate session token with Taruvi", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.ValidateSessionToken", "api.taruvi.session.connection_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.NewAppError("TaruviProvider.ValidateSessionToken", "api.taruvi.session.read_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	rctx.Logger().Info("Taruvi session validation response",
		mlog.Int("status_code", resp.StatusCode),
		mlog.String("response_body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return nil, model.NewAppError("TaruviProvider.ValidateSessionToken", "api.taruvi.session.invalid_token", nil, fmt.Sprintf("Status: %d, Body: %s", resp.StatusCode, string(body)), http.StatusUnauthorized)
	}

	var authResp model.TaruviAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		rctx.Logger().Error("Failed to parse Taruvi session response", mlog.Err(err))
		return nil, model.NewAppError("TaruviProvider.ValidateSessionToken", "api.taruvi.session.unmarshal_error", nil, "", http.StatusInternalServerError).Wrap(err)
	}

	return &authResp, nil
}

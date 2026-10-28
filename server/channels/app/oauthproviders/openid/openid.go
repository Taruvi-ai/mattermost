// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package oauthopenid

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

const (
	discoveryCacheTTL     = time.Hour
	discoveryMaxBodyBytes = 1 << 20
)

type OpenIDProvider struct {
	httpClient *http.Client

	mu        sync.Mutex
	discovery map[string]cachedDiscovery
}

type cachedDiscovery struct {
	doc       discoveryDocument
	fetchedAt time.Time
}

type discoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

type OpenIDUser struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	PreferredUsername string `json:"preferred_username"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	Name              string `json:"name"`
}

func init() {
	einterfaces.RegisterOAuthProvider(model.ServiceOpenid, newOpenIDProvider(&http.Client{Timeout: 10 * time.Second}))
}

func newOpenIDProvider(httpClient *http.Client) *OpenIDProvider {
	return &OpenIDProvider{
		httpClient: httpClient,
		discovery:  map[string]cachedDiscovery{},
	}
}

func (u *OpenIDUser) IsValid() error {
	if u.Sub == "" {
		return errors.New("user sub can't be empty")
	}

	if u.Email == "" {
		return errors.New("user e-mail should not be empty")
	}

	return nil
}

func userFromOpenIDUser(logger mlog.LoggerIFace, u *OpenIDUser, settings *model.SSOSettings) *model.User {
	email := strings.ToLower(u.Email)

	username := strings.Split(email, "@")[0]
	if settings != nil && model.SafeDereference(settings.UsePreferredUsername) && u.PreferredUsername != "" {
		username = strings.Split(u.PreferredUsername, "@")[0]
	}

	firstName, lastName := u.GivenName, u.FamilyName
	if firstName == "" && lastName == "" && u.Name != "" {
		parts := strings.SplitN(u.Name, " ", 2)
		firstName = parts[0]
		if len(parts) == 2 {
			lastName = parts[1]
		}
	}

	authData := u.Sub

	return &model.User{
		Username:      model.CleanUsername(logger, username),
		FirstName:     firstName,
		LastName:      lastName,
		Email:         email,
		EmailVerified: u.EmailVerified,
		AuthData:      &authData,
		AuthService:   model.ServiceOpenid,
	}
}

func (p *OpenIDProvider) GetUserFromJSON(rctx request.CTX, data io.Reader, _ *model.User, settings *model.SSOSettings) (*model.User, error) {
	var oidcUser OpenIDUser
	if err := json.NewDecoder(data).Decode(&oidcUser); err != nil {
		return nil, err
	}

	if err := oidcUser.IsValid(); err != nil {
		return nil, err
	}

	return userFromOpenIDUser(rctx.Logger(), &oidcUser, settings), nil
}

// GetSSOSettings returns a copy of the service settings with the authorization, token and
// userinfo endpoints filled in from the discovery document, because the System Console only
// stores the discovery endpoint for OpenID Connect.
func (p *OpenIDProvider) GetSSOSettings(_ request.CTX, config *model.Config, service string) (*model.SSOSettings, error) {
	sso := config.GetSSOService(service)
	if sso == nil {
		return nil, fmt.Errorf("no SSO settings for service %q", service)
	}

	settings := *sso

	discoveryURL := strings.TrimSpace(model.SafeDereference(settings.DiscoveryEndpoint))
	if discoveryURL == "" {
		return &settings, nil
	}

	doc, err := p.getDiscovery(discoveryURL)
	if err != nil {
		return nil, err
	}

	settings.AuthEndpoint = model.NewPointer(doc.AuthorizationEndpoint)
	settings.TokenEndpoint = model.NewPointer(doc.TokenEndpoint)
	settings.UserAPIEndpoint = model.NewPointer(doc.UserinfoEndpoint)

	return &settings, nil
}

func (p *OpenIDProvider) getDiscovery(discoveryURL string) (discoveryDocument, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, ok := p.discovery[discoveryURL]; ok && time.Since(cached.fetchedAt) < discoveryCacheTTL {
		return cached.doc, nil
	}

	resp, err := p.httpClient.Get(discoveryURL)
	if err != nil {
		return discoveryDocument{}, fmt.Errorf("failed to fetch OpenID discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return discoveryDocument{}, fmt.Errorf("OpenID discovery document returned status %d", resp.StatusCode)
	}

	var doc discoveryDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, discoveryMaxBodyBytes)).Decode(&doc); err != nil {
		return discoveryDocument{}, fmt.Errorf("failed to parse OpenID discovery document: %w", err)
	}

	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.UserinfoEndpoint == "" {
		return discoveryDocument{}, errors.New("OpenID discovery document is missing the authorization, token or userinfo endpoint")
	}

	p.discovery[discoveryURL] = cachedDiscovery{doc: doc, fetchedAt: time.Now()}

	return doc, nil
}

func (p *OpenIDProvider) GetUserFromIdToken(_ request.CTX, _ string) (*model.User, error) {
	return nil, nil
}

func (p *OpenIDProvider) IsSameUser(_ request.CTX, dbUser, oauthUser *model.User) bool {
	return dbUser.AuthData != nil && oauthUser.AuthData != nil && *dbUser.AuthData == *oauthUser.AuthData
}

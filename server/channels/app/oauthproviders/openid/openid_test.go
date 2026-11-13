// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package oauthopenid

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userInfoJSON(t *testing.T, u OpenIDUser) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(u)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func TestGetUserFromJSON(t *testing.T) {
	rctx := request.TestContext(t)
	provider := newOpenIDProvider(http.DefaultClient)

	keycloakUser := OpenIDUser{
		Sub:               "48a9e2f5-2968-400e-bf3a-610f5716dcda",
		Email:             "PrajwalDivate@EOXVantage.com",
		EmailVerified:     true,
		PreferredUsername: "prajwals",
		GivenName:         "Prajwal K",
		FamilyName:        "Divate",
	}

	t.Run("maps standard claims", func(t *testing.T) {
		user, err := provider.GetUserFromJSON(rctx, userInfoJSON(t, keycloakUser), nil, nil)
		require.NoError(t, err)

		assert.Equal(t, keycloakUser.Sub, *user.AuthData)
		assert.Equal(t, model.ServiceOpenid, user.AuthService)
		assert.Equal(t, "prajwaldivate@eoxvantage.com", user.Email)
		assert.True(t, user.EmailVerified)
		assert.Equal(t, "Prajwal K", user.FirstName)
		assert.Equal(t, "Divate", user.LastName)
		assert.Equal(t, "prajwaldivate", user.Username)
	})

	t.Run("uses preferred_username when enabled", func(t *testing.T) {
		settings := &model.SSOSettings{UsePreferredUsername: model.NewPointer(true)}
		user, err := provider.GetUserFromJSON(rctx, userInfoJSON(t, keycloakUser), nil, settings)
		require.NoError(t, err)
		assert.Equal(t, "prajwals", user.Username)
	})

	t.Run("falls back to email when preferred_username is empty", func(t *testing.T) {
		u := keycloakUser
		u.PreferredUsername = ""
		settings := &model.SSOSettings{UsePreferredUsername: model.NewPointer(true)}
		user, err := provider.GetUserFromJSON(rctx, userInfoJSON(t, u), nil, settings)
		require.NoError(t, err)
		assert.Equal(t, "prajwaldivate", user.Username)
	})

	t.Run("splits name when given and family names are missing", func(t *testing.T) {
		u := OpenIDUser{Sub: "abc", Email: "anuj@example.com", Name: "Anuj K Vadecha"}
		user, err := provider.GetUserFromJSON(rctx, userInfoJSON(t, u), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, "Anuj", user.FirstName)
		assert.Equal(t, "K Vadecha", user.LastName)
		assert.False(t, user.EmailVerified)
	})

	t.Run("rejects missing sub", func(t *testing.T) {
		u := keycloakUser
		u.Sub = ""
		_, err := provider.GetUserFromJSON(rctx, userInfoJSON(t, u), nil, nil)
		require.Error(t, err)
	})

	t.Run("rejects missing email", func(t *testing.T) {
		u := keycloakUser
		u.Email = ""
		_, err := provider.GetUserFromJSON(rctx, userInfoJSON(t, u), nil, nil)
		require.Error(t, err)
	})

	t.Run("rejects invalid json", func(t *testing.T) {
		_, err := provider.GetUserFromJSON(rctx, strings.NewReader("not json"), nil, nil)
		require.Error(t, err)
	})
}

func TestGetSSOSettings(t *testing.T) {
	rctx := request.TestContext(t)

	newConfig := func(discoveryURL string) *model.Config {
		cfg := &model.Config{}
		cfg.SetDefaults()
		cfg.OpenIdSettings.DiscoveryEndpoint = model.NewPointer(discoveryURL)
		return cfg
	}

	t.Run("fills endpoints from the discovery document and caches it", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"authorization_endpoint":"https://kc.example/auth","token_endpoint":"https://kc.example/token","userinfo_endpoint":"https://kc.example/userinfo"}`))
		}))
		defer server.Close()

		provider := newOpenIDProvider(server.Client())
		cfg := newConfig(server.URL)

		settings, err := provider.GetSSOSettings(rctx, cfg, model.ServiceOpenid)
		require.NoError(t, err)
		assert.Equal(t, "https://kc.example/auth", *settings.AuthEndpoint)
		assert.Equal(t, "https://kc.example/token", *settings.TokenEndpoint)
		assert.Equal(t, "https://kc.example/userinfo", *settings.UserAPIEndpoint)
		assert.Empty(t, *cfg.OpenIdSettings.AuthEndpoint, "the stored config must not be modified")

		_, err = provider.GetSSOSettings(rctx, cfg, model.ServiceOpenid)
		require.NoError(t, err)
		assert.Equal(t, int32(1), hits.Load())
	})

	t.Run("returns the configured settings when there is no discovery endpoint", func(t *testing.T) {
		provider := newOpenIDProvider(http.DefaultClient)
		cfg := newConfig("")
		cfg.OpenIdSettings.AuthEndpoint = model.NewPointer("https://manual.example/auth")

		settings, err := provider.GetSSOSettings(rctx, cfg, model.ServiceOpenid)
		require.NoError(t, err)
		assert.Equal(t, "https://manual.example/auth", *settings.AuthEndpoint)
	})

	t.Run("errors when the discovery document lacks endpoints", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"issuer":"https://kc.example"}`))
		}))
		defer server.Close()

		provider := newOpenIDProvider(server.Client())
		_, err := provider.GetSSOSettings(rctx, newConfig(server.URL), model.ServiceOpenid)
		require.Error(t, err)
	})

	t.Run("errors when the discovery endpoint does not return 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		provider := newOpenIDProvider(server.Client())
		_, err := provider.GetSSOSettings(rctx, newConfig(server.URL), model.ServiceOpenid)
		require.Error(t, err)
	})
}

func TestIsSameUser(t *testing.T) {
	rctx := request.TestContext(t)
	provider := newOpenIDProvider(http.DefaultClient)

	dbUser := &model.User{AuthData: model.NewPointer("sub-1")}

	assert.True(t, provider.IsSameUser(rctx, dbUser, &model.User{AuthData: model.NewPointer("sub-1")}))
	assert.False(t, provider.IsSameUser(rctx, dbUser, &model.User{AuthData: model.NewPointer("sub-2")}))
	assert.False(t, provider.IsSameUser(rctx, dbUser, &model.User{}))
}

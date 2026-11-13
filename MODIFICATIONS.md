# Modifications

This repository is a modified version of [Mattermost](https://github.com/mattermost/mattermost), based on the `v11.7.11` release. Taruvi made the changes below. The licensing in [LICENSE.txt](LICENSE.txt) applies to the whole work, including these changes.

## Modified Mattermost files

| File | Change | Last modified |
| --- | --- | --- |
| `server/channels/app/limits.go` | User limits for servers without a license raised to 30,000 | 2026-10-05 |
| `server/channels/app/oauth.go` | First OpenID Connect login links an existing account by verified email | 2026-10-29 |
| `server/channels/app/oauth_test.go` | Tests for OpenID Connect account linking | 2026-10-29 |
| `server/cmd/mattermost/main.go` | Registers the OpenID Connect provider | 2026-10-29 |
| `server/config/client.go` | OpenID Connect login settings sent to clients without a license | 2026-11-12 |
| `server/config/client_test.go` | Test for OpenID Connect settings without a license | 2026-11-12 |
| `webapp/channels/src/components/about_build_modal/about_build_modal.tsx` | Link to this repository's source for the running build | 2026-11-12 |
| `webapp/channels/src/components/global_header/left_controls/product_menu/` | Edition badge and edition menu item removed from the product menu | 2026-11-12 |
| `webapp/channels/src/components/header_footer_route/header.tsx`, `header.scss` | Edition badge removed from the login page header | 2026-11-12 |
| `webapp/channels/src/i18n/en.json` | Text for the source code link | 2026-11-12 |

## Files added by Taruvi

| File | Purpose | Last modified |
| --- | --- | --- |
| `Dockerfile` | Builds the server and webapp from this repository on top of the Team Edition package | 2026-11-13 |
| `server/channels/app/oauthproviders/openid/` | OpenID Connect provider | 2026-10-29 |
| `.github/workflows/harbor-push.yml`, `deploy/` | Build and deployment for Taruvi | 2026-11-13 |

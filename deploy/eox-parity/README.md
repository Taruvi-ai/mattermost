# EOX chat → Taruvi chat: settings replication

What was customized on the live EOX Vantage chat (`chatprod.eoxvantage.com`, Mattermost 9.6.0), how it compares with Taruvi chat today (11.x), and how the files in this folder replicate it.

Audited read-only on 8 Oct 2026. Nothing has been applied yet.

## Summary

- **Permissions.** EOX restricts who can create teams and channels and who can manage public channels. Taruvi has none of these restrictions; any member can create teams and channels.
- **Server settings.** The patch copies 14 EOX settings; upload size and push server are handled separately. Production has none of the 14. Staging already has 5 through Infisical.
- **Teams and integrations.** These move with the database and S3 copy. Nothing on EOX needs a Mattermost license.
- **Files.** `config-patch.json` holds the settings and `apply.sh` applies them plus the permission rules.

## Where settings live

| Kind | EOX live | Taruvi chat | Comes with a database copy? |
| --- | --- | --- | --- |
| Permissions | Roles in the database | Roles in the database | Yes |
| Server settings | `config.json` file in the image | Database config (`MM_CONFIG`) + Infisical env in the Mattermost resource; env wins | **No** |
| Teams, admins, bots, webhooks, emoji | Database + S3 | Database + S3 | Yes |

## 1. Permissions

| Action | EOX live | Taruvi today | After `apply.sh` |
| --- | --- | --- | --- |
| Create public channels | Team admins, system admins | Every member | Team admins, system admins |
| Create private channels | Team admins, system admins | Every member | Team admins, system admins |
| Create teams | System admins | Every member | System admins |
| Rename a public channel, edit header and purpose | Team admins, system admins | Every member | Team admins, system admins |
| Archive a public channel | Team admins, system admins | Every member | Team admins, system admins |
| Add or remove other people in a public channel | System admins only | Every member | Team admins, channel admins, system admins |

The last row differs from EOX on purpose: team and channel admins can manage public channel members. Anyone can still join a public channel themselves.

**Same on both, no change needed:**

- Private channels: any member of the channel can rename, archive, and add or remove people.
- Any team member can add existing users to their team.
- Members edit and delete their own posts. Team admins delete others' posts.
- Only team admins convert channels between public and private.
- Any member can create custom emoji.

These can also be edited by hand in **System Console > User Management > Permissions > System Scheme**. That works without a license on 11.7.

## 2. Server settings

### Copied by `config-patch.json`

| Setting | EOX | Taruvi prod | Taruvi staging | Why |
| --- | --- | --- | --- | --- |
| `TeamSettings.MaxUsersPerTeam` | 5,000 | **50** | 5,000 (env) | EOX Vantage team has 2,258 members |
| `TeamSettings.MaxNotificationsPerChannel` | 1,000,000 | 1,000 | 1,000,000 (env) | Above this, @all / @channel / @here stop notifying |
| `TeamSettings.MaxChannelsPerTeam` | 20,000 | 2,000 | 20,000 (env) | Parity; main team has 740 channels |
| `TeamSettings.TeammateNameDisplay` | `full_name` | `username` | `full_name` (env) | People shown by full name |
| `ServiceSettings.EnableUserAccessTokens` | true | **false** | true | 51 tokens (trackit, crmapp, qms-bot) stop working when off |
| `ServiceSettings.EnableBotAccountCreation` | true | false | false | 47 bot accounts |
| `ServiceSettings.EnablePostUsernameOverride`, `EnablePostIconOverride` | true | false | false | 20 incoming webhooks post under their own name and icon |
| `ServiceSettings.TrustedProxyIPHeader` | `X-Forwarded-For`, `X-Real-IP` | empty | empty | Real client IPs in logs and rate limits behind ALB + nginx |
| `ServiceSettings.EnableTutorial`, `EnableOnboardingFlow` | false | true | true | No first-run tours for existing users |
| `ServiceSettings.ExperimentalEnableDefaultChannelLeaveJoinMessages` | false | true | true | No "X joined the channel" posts |
| `AnnouncementSettings.UserNoticesEnabled` | false | true | true | No in-product notices |
| `EmailSettings.EnablePreviewModeBanner` | false | true | true | No "email not configured" banner |
| `ImageProxySettings.Enable` (local) | true | false | false | Server fetches external images |
| NPS survey plugin | off | on | on | No survey bot messages |

### Decided (differs from EOX)

| Setting | EOX | Decision |
| --- | --- | --- |
| `ServiceSettings.CollapsedThreads` | `disabled` | `always_on` (in the patch). Staging pins `disabled` in Infisical; change it there |
| `TeamSettings.SiteName` | EOX Vantage | Taruvi (in the patch) |
| Custom brand, support links, app download links, email sender | EOX Vantage values | Not copied; Taruvi values still to be chosen |

### Copy, but outside the patch

| Setting | EOX | Taruvi | What to do |
| --- | --- | --- | --- |
| Upload size | 200 MB effective (ingress `200m`) | 100 MB | Set `proxy-body-size` to `200m` in `deploy/mattermost-production.yaml`; the operator derives `MaxFileSize` from it |
| `EmailSettings.PushNotificationServer` | `chatpushproxy.eoxvantage.com` | prod: none (push off), staging: `push-test.mattermost.com` | Needed for the EOX mobile apps; set once the mobile app decision is made |

### Still to decide

| Setting | EOX | Taruvi | Question |
| --- | --- | --- | --- |
| `EmailSettings.SendEmailNotifications` | false | true, but SMTP is `localhost` | Configure SMTP or turn email off |
| `ServiceSettings.EnableAPIUserDeletion`, `EnableAPITeamDeletion` | true | false | Does EOS provisioning permanently delete users or teams? |
| `ServiceSettings.EnableInsecureOutgoingConnections` | true | false | Skips TLS checks; leave off unless an integration breaks |
| `SqlSettings.QueryTimeout` | 500 s | 30 s | Raised on EOX for search over 16.6M posts |
| Plugins | see below | | Which plugins move |

### Do not copy

| Setting | Reason |
| --- | --- |
| `PasswordSettings.MinimumLength` = 5 | Only for EOS auto-created passwords |
| `ServiceSettings.AllowCorsFrom` = `*` | Taruvi uses an explicit list |
| `ServiceSettings.EOSLoginUrl`, `UserProfileUrl` | EOS sign-in, replaced by the identity decision |
| `BleveSettings.*` | Bleve was removed in 11.x; search uses the database |
| Debug logging, `EnableClientPerformanceDebugging`, `EnableAPITriggerAdminNotifications` | Debugging leftovers |
| S3, database, salts, at-rest key | Infrastructure; Taruvi has its own |

Already the same on both: 180-day sessions without extension, open sign-up off, email invitations, developer mode, LaTeX, SVGs. Guest accounts and public file links are off on EOX.

## 3. Teams, roles and integrations

These move with the database and S3 copy.

- **Teams:** 122 active, all invite-only, no email-domain restrictions, 12 custom icons. 7 have no team admin; 29 have one member or none.
- **System roles:** 17 system admins, 5 users who can post to every channel, 1 user allowed to create tokens.
- **Integrations:** 20 incoming webhooks, 47 bots, 51 personal access tokens, 1 OAuth app, 258 custom emoji.
- **Not used on EOX:** permission schemes, team overrides, channel moderation, terms of service, custom user groups.

## 4. Plugins

Plugin on/off state and settings are server config, so they don't come with a database copy. Plugin data does.

- **On in EOX:** matterpoll, remind, memes, profanity filter, file list, copy post, custom attributes, draw, Google Calendar, screen, walltime, appbot (EOX custom).
- **On in Taruvi but off in EOX:** Calls, Playbooks, NPS. The patch turns NPS off.

## 5. How to apply

1. **Review and commit** this folder. Branch `chore/eox-parity-settings` is local, from `dev`, not pushed.
2. **Staging prerequisites in Infisical:**
   - `MM_SERVICESETTINGS_ENABLELOCALMODE=true`. It's `false` today, and `apply.sh` needs local mode.
   - `MM_SERVICESETTINGS_COLLAPSEDTHREADS=always_on`.
3. **Apply to staging:** `deploy/eox-parity/apply.sh mattermost-staging`.
4. **Test on staging** with three users:
   - **Regular member:** can't create a team or channel, or rename, archive or add people to a public channel.
   - **Team admin:** can create, rename and archive channels.
   - **Webhook and bot:** a webhook posts under its own name, and a bot token works.
5. **Production:**
   - Raise the ingress body size to `200m`.
   - Set the push server once mobile is decided.
   - Run `deploy/eox-parity/apply.sh mattermost-production`. Local mode is already on there.

`apply.sh` gives the same result whether it runs on the current Taruvi roles or on a copied EOX database. It ends by printing the roles so you can check them.

> **Watch out:** server settings are stored in whichever database `MM_CONFIG` points at. If production's `MM_CONFIG` is switched to a copied EOX database, the server starts on defaults, because EOX never stored config in its database. Either keep `MM_CONFIG` on `mattermost_taruviprod` or run `apply.sh` again right after switching.

## Files

| File | What it does |
| --- | --- |
| `config-patch.json` | Settings merged into the stored config with `mmctl config patch`. Only listed keys change |
| `apply.sh <installation>` | Applies the patch and the permission rules inside the pod, then prints the roles |

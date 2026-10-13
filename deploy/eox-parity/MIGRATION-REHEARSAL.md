# EOX chat → chatstag: migration rehearsal

Plan for loading a copy of live EOX chat (`chatprod.eoxvantage.com`, Mattermost 9.6.0) into staging (`chatstag.taruvi.app`, 11.7.11) so QC can test it. The rehearsal also times each step for the real cutover.

Researched read-only on 8 Oct 2026. Nothing has been done yet.

## Short answers

| Question | Answer |
| --- | --- |
| Clone the production database into chatstag? | Yes, as a **new database** beside chatstag's current one, owned by a **new database user**. chatstag's current database stays untouched for rollback. |
| Restart the server so the migrations run? | **Migrate first, then switch.** chatstag's health check restarts a pod that doesn't answer within about 40 seconds, which would kill a long migration. Run `mattermost db migrate` once in a separate pod, then point chatstag at the upgraded clone. |
| Point chatstag at production's S3? | **No.** Copy the files into the staging bucket, and before mid-January 2027: files from before April 2024 are in Glacier Deep Archive and become unreadable around 31 January. See [Files](#files). |
| Clone and mount the search index volume? | **No.** Mattermost 11.0 removed Bleve search. 11.x searches the database, and the full-text indexes come with the database clone. |

## What the research found

**The upgrade itself looks low-risk.**

- EOX is at schema migration 121; 11.7.11 is at 195. Of the 50 migrations in between, none rewrites or re-indexes the posts table. The heaviest builds two statistics views by scanning all 16.6M posts.
- EOX's database was converted from MySQL with pgloader, but its schema matches what Mattermost creates itself, column for column and index for index. Every difference is one of the 50 pending migrations or a plugin table. The full-text search indexes are present.
- Mattermost recommends upgrading one long-term-support (ESR) release at a time, mainly so mixed-version clusters keep working during rolling upgrades. chatstag is one pod upgraded with downtime, so a direct 9.6 → 11.7 jump is reasonable. This rehearsal is what proves it.
- Mattermost 11.7 without a license caps active users at 250. The Taruvi fork raised the cap to 30,000. EOX has about 1,290 active accounts, so this only works with the Taruvi image.

**The environment needs care.**

- chatstag's database (`mattermost_chatstag`) lives on `prod-postgres`, the same RDS instance as live EOX chat and about 30 other databases. It's a db.m8g.large (2 vCPU, 8 GB) with 130 GB free; CPU averages 18% but peaked at 99% this week.
- chatstag and EOX both connect as the RDS master user `postgres`.
- chatstag has three database connections:

| Setting | Where it's set | Used for |
| --- | --- | --- |
| `MM_SQLSETTINGS_DATASOURCE` | Infisical → `mm-infisical` | Writes and most reads |
| `MM_SQLSETTINGS_DATASOURCEREPLICAS` | `mm-db-secret` | Reads routed to "replicas" (same database today) |
| `MM_CONFIG` | `mm-db-secret` (`DB_CONNECTION_STRING`) | Server settings |

- Every push to `dev` rebuilds `mm-infisical` from Infisical and redeploys chatstag. The deploy fails if the pod isn't ready within 10 minutes.
- chatstag today holds 87 users, 1 team and 363 posts (40 MB).
- chatstag never checks Mattermost passwords. Each login is verified by Keycloak (`taruvi_new`) or Taruvi, then matched to a Mattermost user by email or username.

## Things to be careful about

1. **Touching live EOX's database.** If chatstag's data connection pointed at `mattermost_prod` instead of the clone, the 11.7 server would migrate live EOX's database while EOX's 9.6 server is still using it, with no supported way back. Use a new database user that can only reach the clone, and check the database name inside the pod before anyone logs in.
2. **Load on the shared RDS instance.** Decided: the clone goes on `prod-postgres`, so timings match the real cutover. Dump, restore and migrate in the lowest-traffic window and watch CPU.
3. **Reads and writes going to different databases.** `MM_SQLSETTINGS_DATASOURCE` and `MM_SQLSETTINGS_DATASOURCEREPLICAS` must both point at the clone. If only one moves, chatstag writes to one database and reads from the other. `MM_CONFIG` stays on `mattermost_chatstag`, which keeps chatstag's server settings.
4. **CI undoing the switch.** Put the clone's connection string in Infisical, not only in the Kubernetes secret. Otherwise the next push to `dev` switches chatstag back.
5. **Contacting real people.** The clone holds real email addresses and EOX users' logged-in mobile devices, and staging sends full message text in push notifications. Turn email and push off on staging, and delete the cloned sessions. That also invalidates every session copied from production.
6. **Who can read EOX messages.** chatstag is public, and the clone has about 1,290 people's direct messages. Anyone with a Keycloak or Taruvi staging account whose email matches an EOX user gets in as that user. Make sure neither allows sign-up without email verification, get approval to use production data on staging, and delete the copy after QC.
7. **Plugins from the file store.** EOX's files include plugin bundles from 2022–2025 in `prod/plugins/`. Mattermost installs any bundle it finds there at startup. Don't copy that folder. Install the plugins we choose, in versions built for 11.x.
8. **Taruvi staging users lose chat.** During the rehearsal chatstag only has EOX's accounts. The 87 Taruvi staging users and the chat embedded in Taruvi staging sites stop working until rollback. Tell the team beforehand.
9. **Search speed.** Every search now runs on Postgres. EOX raised the query timeout to 500 s; chatstag uses 30 s. Measure before choosing a value.

## Files

### Most of EOX's files are in Glacier Deep Archive

An S3 Batch Operations job in May 2024 moved everything in `prod/` older than mid-April 2024 to Glacier Deep Archive. Every archived object sampled (attachments, emoji, a team icon) opens today only because it was restored temporarily, and **those restored copies expire between 31 January and 3 February 2027**.

| In `prod/` | Deep Archive (restored until ~31 Jan 2027) | Standard |
| --- | --- | --- |
| Attachments, thumbnails, previews | 3.33M objects, 2019 to mid-April 2024 (≈512 GB of originals) | 2.58M objects, mid-April 2024 on (≈543 GB) |
| Custom emoji images | 193 | 132 |
| Profile pictures | 864 | 1,982 |
| Team icons | 13 | 1 |

After the restored copies expire, those files stop loading on live EOX, and they can't be copied until they're restored again. A new restore takes up to 48 hours. **Copy everything before mid-January 2027.** A copy lands in standard storage, so files copied to Taruvi's buckets stay readable after the deadline. This applies to the production bucket too, not just the rehearsal.

### Copy instead of sharing

Pointing chatstag at `s3://eoschat/prod/` would cause three problems:

- chatstag would write into live EOX's bucket: uploads, thumbnails, profile pictures, emoji and plugin bundles. Live EOX installs bundles from `prod/plugins/` when it restarts, so staging's 11.x plugins could end up on live EOX.
- Permanent deletes on staging would delete live EOX's files.
- chatstag would need EOX's `s3-www3` key, which has full S3 access to the whole EOX AWS account. The two buckets are in different AWS accounts.

Copy instead:

| | |
| --- | --- |
| **From** | `s3://eoschat/prod/` (EOX account `150356453157`): 2,709 date folders, `users/`, `teams/`, `emoji/`. Skip `plugins/` and `brand/`. |
| **To** | `s3://mattermost-chatstag/eox-rehearsal/` (Taruvi account `849882217263`, us-east-2). The bucket enforces bucket-owner ownership, so chatstag can read objects copied in from the EOX account. |
| **Size** | About 5.9M objects and 1.1 TB. Roughly US$25 a month in standard storage. |
| **Access** | Temporary bucket policy on `mattermost-chatstag` that lets the copying identity in the EOX account list and write under `eox-rehearsal/`. Remove it after the copy. |
| **How** | `aws s3 sync s3://eoschat/prod/ s3://mattermost-chatstag/eox-rehearsal/ --exclude 'plugins/*' --exclude 'brand/*' --force-glacier-transfer --sse AES256`, run inside AWS in us-east-2 so S3 copies server-side. Without `--force-glacier-transfer` the CLI skips the archived half. Expect hours; start days early. Run it once more after the database dump so every file the clone references exists. |
| **chatstag** | Set `MM_FILESETTINGS_AMAZONS3PATHPREFIX=eox-rehearsal`. chatstag's own files at the bucket root stay untouched. |

## Steps

### 0. Before the day

- [ ] Approval to put production data on staging, a list of who gets access, and a deletion date.
- [ ] Keycloak `taruvi_new` and Taruvi staging don't allow sign-up without a verified email.
- [ ] Pick the EOX users QC will test as (regular member, team admin, system admin) and give QC matching Keycloak accounts.
- [ ] Infisical staging values:
  - `MM_SERVICESETTINGS_ENABLELOCALMODE=true` (needed by `apply.sh`)
  - `MM_SERVICESETTINGS_COLLAPSEDTHREADS=always_on`
  - `MM_EMAILSETTINGS_SENDEMAILNOTIFICATIONS=false`
  - `MM_EMAILSETTINGS_SENDPUSHNOTIFICATIONS=false`
- [ ] Tell the Taruvi team chatstag will hold EOX data only.
- [ ] Start the S3 copy.

### 1. Clone the database (low-traffic window)

As the master user, once:

```sql
CREATE ROLE mm_eox_rehearsal LOGIN PASSWORD '<new secret>';
CREATE DATABASE mattermost_chatstag_eox OWNER mm_eox_rehearsal;
```

From a pod in the cluster with the PostgreSQL 18 client and a 50 GiB volume:

```bash
pg_dump -h <prod-postgres> -U postgres -d mattermost_prod -Fd -j 2 --no-owner --no-acl -f /dump/eox
pg_restore -h <prod-postgres> -U mm_eox_rehearsal -d mattermost_chatstag_eox -j 2 --no-owner --no-acl /dump/eox
```

Then, in the clone:

```sql
DELETE FROM sessions;
```

Record the row counts for users, posts, file records, channels and teams, and the time each command took.

### 2. Run the migrations (chatstag still on its own database)

Run a one-off pod:

- **Image:** `repo.eoxvantage.com/taruvi/mattermost:<tag chatstag runs>`, pull secret `registrypullsecret`.
- **Env:** `MM_SQLSETTINGS_DRIVERNAME=postgres`, plus `MM_SQLSETTINGS_DATASOURCE` set to the clone, read from a temporary secret.
- **Command:** `mattermost db migrate`. No health checks on this pod.

Then run `mattermost db version`; it should print 195. Record the time.

If the migration fails, read the error and check the `db_migrations` and `db_lock` tables. Restore the clone again, then either retry with `--auto-recover` or step through stock 9.11 and 10.11 images first.

### 3. Switch chatstag

1. Infisical staging: set `MM_SQLSETTINGS_DATASOURCE` to the clone, and apply the same value to `mm-infisical` with `kubectl` (CI only syncs on the next push to `dev`).
2. `mm-db-secret`: set `MM_SQLSETTINGS_DATASOURCEREPLICAS` to the clone. Leave `DB_CONNECTION_STRING` on `mattermost_chatstag`.
3. Add `MM_FILESETTINGS_AMAZONS3PATHPREFIX=eox-rehearsal` to `spec.mattermostEnv` on `mattermost-staging`. CI only patches the version, so it stays.
4. Restart chatstag. In the pod, confirm both data connections name `mattermost_chatstag_eox` and `MM_CONFIG` names `mattermost_chatstag`.
5. Watch the first start. It runs the startup data migrations (new permissions, empty-draft cleanup, invalid DM preferences) before it starts serving.
6. Run `deploy/eox-parity/apply.sh mattermost-staging`. It must run after the first start, because that start adds new permissions to the roles.

### 4. Checks before QC

- [ ] **Data:** row counts match step 1, schema version is 195, no repeating errors in the logs.
- [ ] **Login:** each QC account, from the chatstag login page and from a Taruvi staging site.
- [ ] **Content:** the EOX Vantage team (2,258 members, 740 channels) loads. A channel scrolls back to 2019. Direct and group messages and threads open.
- [ ] **Files:** attachments from 2019, 2023 and this month open, with thumbnails and previews. Profile pictures, team icons and custom emoji show.
- [ ] **Search:** words, phrases, `from:`, `in:`, date ranges. Note how long each takes and whether any hit the 30 s timeout.
- [ ] **Permissions:** the three-user test in the README, section 5.
- [ ] **Integrations:** a cloned incoming webhook posts when called on the chatstag URL, and a bot token works against the API.
- [ ] **Nothing goes out:** email and push are off in the System Console.
- [ ] **Load:** RDS CPU while the checks run.

### 5. Hand to QC

Give QC the accounts, the checks above, and the known gaps:

- EOS sign-in and EOX mobile push aren't wired; both wait on the identity and mobile decisions.
- EOX's plugins (polls, reminders, memes, Google Calendar and others) aren't installed unless we pick them.
- Boards and Playbooks don't run on 11.x without a license.
- Branding is Taruvi's.
- Files uploaded to live EOX after the final file sync are missing.

### 6. After QC

- **Rollback:** point both data connections back to `mattermost_chatstag`, remove the path prefix, restart.
- **Clean up:** drop `mattermost_chatstag_eox` and `mm_eox_rehearsal`, delete `eox-rehearsal/` from the bucket, the dump volume, and the temporary bucket policy.
- **Keep the timings:**

| Step | Time |
| --- | --- |
| Database dump | |
| Database restore | |
| `mattermost db migrate` | |
| First start | |
| File sync (full, then final) | |

## Decisions needed

| Question | Options |
| --- | --- |
| Who owns the Deep Archive restore on live EOX | Someone at EOX has to restore again before ~31 Jan 2027, or live EOX loses pre-April-2024 files if it's still running then |
| Which EOX users QC logs in as | Names and roles |
| Mobile in this round | Yes needs the EOX push proxy and app decision; no keeps push off |

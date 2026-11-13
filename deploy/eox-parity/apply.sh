#!/usr/bin/env bash
# Applies the EOX chat settings and permission rules to a Taruvi Mattermost installation.
# Needs local mode on the server (ServiceSettings.EnableLocalMode) and kubectl access to the cluster.
# Settings pinned by env in the Mattermost resource (Infisical) override this patch and must be changed there.
#
# Usage: deploy/eox-parity/apply.sh mattermost-staging|mattermost-production
set -euo pipefail

installation="${1:?usage: $0 mattermost-staging|mattermost-production}"
namespace=mattermost
dir="$(cd "$(dirname "$0")" && pwd)"

pod="$(kubectl get pod -n "$namespace" -l "installation.mattermost.com/installation=$installation" -o jsonpath='{.items[0].metadata.name}')"
mm() { kubectl exec -n "$namespace" "$pod" -c mattermost -- mmctl --local "$@"; }

kubectl exec -i -n "$namespace" "$pod" -c mattermost -- sh -c 'cat > /tmp/eox-parity-config-patch.json' < "$dir/config-patch.json"
mm config patch /tmp/eox-parity-config-patch.json

# Only team admins and system admins create channels; only system admins create teams.
mm permissions remove system_user create_team
mm permissions remove team_user create_public_channel create_private_channel
mm permissions add team_admin create_public_channel create_private_channel delete_public_channel manage_public_channel_properties manage_public_channel_members
# Regular members can't rename, archive, or manage members of public channels; channel and team admins still manage members.
mm permissions remove channel_user delete_public_channel manage_public_channel_properties manage_public_channel_members
mm permissions add channel_admin manage_public_channel_members

for role in system_user team_user channel_user channel_admin team_admin; do
  mm permissions role show "$role"
done

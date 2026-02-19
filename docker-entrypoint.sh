#!/bin/bash
set -e

# If MM_TARUVISETTINGS_TARUVISERVERURL is set, delete config.json to force env var usage
if [ -n "$MM_TARUVISETTINGS_TARUVISERVERURL" ] && [ -f /mattermost/config/config.json ]; then
    echo "Removing config.json to use environment variables"
    rm -f /mattermost/config/config.json
fi

exec /mattermost/bin/mattermost "$@"

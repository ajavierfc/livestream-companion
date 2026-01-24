#!/bin/bash
set -e
destination="$1"
[ ! "$destination" ] && echo "No destination supplied. Syntax: $0 <destination>" && exit 1
rsync -Pazv livestream-companion auth-proxy-service update_playlists.sh run.sh ui $destination
echo -n "Copy data folder? [yN]:"; read yes
[ "y" == "${yes,}" ] && rsync -Pazv data $destination

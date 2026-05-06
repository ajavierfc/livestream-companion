#!/bin/bash
set -e
cd $(dirname $0)
destination="$1"
[ ! "$destination" ] && echo "No destination supplied. Syntax: $0 <ssh-destination:/livestream-root-path>" && exit 1
rsync -Pazv livestream-companion auth-proxy-service update_playlists.sh update_geoip.sh run.sh ui $destination
echo -n "Copy data folder? [yN]:"; read yes
[ "y" == "${yes,}" ] && rsync -Pazv data $destination

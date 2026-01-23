#!/bin/bash
cd $(dirname $0)

ETH_DEVICE=`ip link | grep ^2: | awk -F "[: ]+" '{ print $2 }'`
if [[ `ip route list dev $ETH_DEVICE` =~ src\ ([0-9.]+) ]]; then
  export ETH_IPV4=${BASH_REMATCH[1]}
else
  export ETH_IPV4="127.0.0.1"
fi

(timeout 1 bash -c '</dev/tcp/$ETH_IPV4/5004') 2> /dev/null || (echo "Port for livestream-companion isn't reachable"; exit 1)

echo "Using gateway IP $ETH_IPV4 on device $ETH_DEVICE"

curl -s "http://$ETH_IPV4:5004/api/playlists" | jq -c .[] | while read -r obj; do
    id=$(echo "$obj" | jq -r '.ID')    
    status=$(echo "$obj" | jq -r '.ImportStatus')
    [ "$status" == "1" ] && echo "Already importing playlist $id" && continue
    echo "Updating playlist with ID: $id"
    curl -s -X PUT "http://$ETH_IPV4:5004/api/playlist/$id" \
        -H "Content-Type: application/json" \
        -d "$obj" > /dev/null
    while [ $status -ne 1 ]; do
      status=`curl -s "http://$ETH_IPV4:5004/api/playlist/3" | jq -r .ImportStatus`
      sleep 5
    done
    while [ $status -eq 1 ]; do
      status=`curl -s "http://$ETH_IPV4:5004/api/playlist/3" | jq -r .ImportStatus`
      sleep 10
    done
done

#!/bin/bash
cd $(dirname $0)

ETH_DEVICE=`ip link | grep ^2: | awk -F "[: ]+" '{ print $2 }'`
if [[ `ip route list dev $ETH_DEVICE` =~ src\ ([0-9.]+) ]]; then
  export ETH_IPV4=${BASH_REMATCH[1]}
else
  export ETH_IPV4="127.0.0.1"
fi
HOST=$ETH_IPV4:5004

if ! (timeout 1 bash -c '</dev/tcp/$ETH_IPV4/5004') 2> /dev/null; then
    echo "$HOST not reachable"
    exit 1
fi

echo "Using gateway host $HOST found on $ETH_DEVICE"

curl -s "http://$HOST/api/playlists" | jq -c .[] | while read -r obj; do
    id=$(echo "$obj" | jq -r '.ID')    
    status=$(echo "$obj" | jq -r '.ImportStatus')
    [ "$status" == "1" ] && echo "Already importing playlist $id" && continue
    echo -en "\nUpdating playlist with ID: $id "
    curl -s -X PUT "http://$HOST/api/playlist/$id" \
        -H "Content-Type: application/json" \
        -d "$obj" > /dev/null
    while [ $status -ne 1 ]; do
      status=`curl -s "http://$HOST/api/playlist/$id" | jq -r .ImportStatus`
      sleep 5
    done
    while [ $status -eq 1 ]; do
      status=`curl -s "http://$HOST/api/playlist/$id" | jq -r .ImportStatus`
      sleep 10
      echo -n '.'
    done
done

echo -e "\ndone"

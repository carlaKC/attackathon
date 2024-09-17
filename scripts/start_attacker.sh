#!/bin/bash

if [ ! -d "attackathon" ]; then
    echo "Error: attackathon repo not found, script should be run from directory containing it."
    exit 1
fi

# Check if the correct number of arguments are provided
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <network_name>"
    exit 1
fi

# Get our target alias from the data file where it's stored.
network_name="$1"

current_directory=$(pwd)
sim_files="$current_directory/attackathon/data/$network_name"
target_alias=$(cat $sim_files/target.txt)

echo "Adding attacking nodes to cluster"
kubectl apply -f attackathon/setup/armada.yaml

while true; do
    # Get the status of pods in the namespace
    pod_status=$(kubectl get pods -n warnet-armada --no-headers)

    # Check if all pods are ready
    if [[ $(echo "$pod_status" | grep -c -E '\s[0-9]+\/[0-9]+\s+Running\s+') -eq $(echo "$pod_status" | wc -l) ]]; then
        echo "All pods are ready."
        break
    else
        echo "Waiting for attacking nodes to be ready"
        sleep 1
    fi
done

echo "Copying in attacking node credentials"
./attackathon/scripts/credentials.sh

target_info=$(warcli lncli $target_alias getinfo)
target_pubkey=$(echo "$target_info" | jq -r '.identity_pubkey')

echo "Setting target node pubkey: $target_pubkey in target.txt"
kubectl exec -it flagship -n warnet-armada -- bash -c 'echo '$target_pubkey' > /target.txt'

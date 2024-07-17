#!/bin/bash

# Check if the 'warnet' directory exists
if [ ! -d "warnet" ]; then
    echo "Error: Warnet directory not found. Make sure to clone Warnet before running this script."
    exit 1
fi

# Check if the correct number of arguments are provided
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <network_name>"
    exit 1
fi

network_name="$1"

# Capture the current working directory, which has the attackathon files in it
current_directory=$(pwd)
sim_files="$current_directory/attackathon/data/$network_name"

cd warnet
pip install -e .

echo "💣 Bringing up warnet 💣"
warcli network start "$sim_files/$network_name.graphml" --force

check_status() {
    warcli network status | grep -q "pending"
}

# Loop until the status does not contain "pending"
while check_status; do
    echo "Waiting for network to come up"
    sleep 5
done

echo "Waiting for L1 p2p connections"
while warcli network connected | grep -q "False"; do
    sleep 1
done

echo "Opening channels and syncing gossip"
warcli scenarios run ln_init

echo "Waiting for gossip to sync"
while warcli scenarios active | grep -q "True"; do
    sleep 1
done

echo "Mining blocks every 5 minutes"
warcli scenarios run miner_std --allnodes --interval=300

# Sim-ln should already be running if we added it in the graph file, export creds here.
echo "Generating random payment activity"
target_alias=$(cat $sim_files/target.txt)
warcli network export --exclude="[$target_alias]"

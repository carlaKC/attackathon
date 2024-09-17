#!/bin/bash

# Path to the nodes.txt file
nodes_file="credentials/nodes.txt"

# Check if nodes.txt exists
if [ ! -f "$nodes_file" ]; then
    echo "Error: nodes.txt file not found."
    exit 1
fi

lncli0="lncli --network=regtest --tlscertpath=/credentials/lnd0-tls.cert --macaroonpath=/credentials/lnd0-admin.macaroon --rpcserver=lightning-0.warnet-armada "
lncli1="lncli --network=regtest --tlscertpath=/credentials/lnd1-tls.cert --macaroonpath=/credentials/lnd1-admin.macaroon --rpcserver=lightning-1.warnet-armada "
lncli2="lncli --network=regtest --tlscertpath=/credentials/lnd2-tls.cert --macaroonpath=/credentials/lnd2-admin.macaroon --rpcserver=lightning-2.warnet-armada "

# Iterate through each line in nodes.txt
while IFS= read -r line; do
    echo "Connecting nodes to $line"
    # Call lncli connect with the extracted string
    $lncli0 connect "$line"
    $lncli1 connect "$line"
    $lncli2 connect "$line"
done < "$nodes_file"

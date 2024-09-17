#!/bin/bash

# Check if the correct number of arguments are provided
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <network_name>"
    exit 1
fi

network_name="$1"

echo "Tearing down previous network"
warcli network down

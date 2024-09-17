#!/bin/bash

# Check if n is provided
if [ -z "$1" ]; then
  echo "Usage: $0 n"
  exit 1
fi

# Read the argument
n=$1

# Loop from 0 to n
for ((i=0; i<n; i++))
do
  # Format i to be zero-padded to 6 digits
  padded_i=$(printf "%06d" $i)
  
  # Execute the kubectl command and save the output to i_reputation_thresholds.json
  kubectl -n warnet exec -it warnet-tank-ln-$padded_i -c ln-cb -- wget -qO- http://localhost:9235/api/reputation_thresholds > ${padded_i}_reputation_thresholds.json
done

echo "Done. Created files from 000000_reputation_thresholds.json to $(printf "%06d" $n)_reputation_thresholds.json"

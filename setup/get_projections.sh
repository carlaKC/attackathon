#!/bin/bash

if [ ! -d "sim-ln" ]; then
    echo "Error: Sim-LN directory not found. Make sure to clone Sim-LN before running this script."
    exit 1
fi

# Check if required arguments are provided
if [ $# -gt 1 ]; then
    echo "Network required for projection generation"
    exit 1
fi

# Get the location we'll put the results
network_name="$1"
current_directory=$(pwd)

# Get simulation file for selected network.
sim_files="$current_directory"/attackathon/data/"$network_name"
simfile="$sim_files"/simln.json

# Create directory for projected revenue.
projection_files="$sim_files"/projections
mkdir -p "$projection_files"

cd sim-ln
if [[ -n $(git status --porcelain) ]]; then
    echo "Error: there are unsaved changes in sim-ln, please stash them!"
    exit 1
fi

git remote add carla https://github.com/carlaKC/sim-ln

git fetch carla > /dev/null 2>&1 || { echo "Failed to fetch carla"; exit 1; }
git checkout carla/attackathon > /dev/null 2>&1 || { echo "Failed to checkout carla/attackathon"; exit 1; }

echo "Installing sim-ln for data generation"
cargo install --locked --path sim-cli

git remote remove carla
git checkout main > /dev/null 2>&1

# Next, generate data that we'll use to project the payments that the network would make without an attack 
# present. We do this for a fixed 1 week period, because we're unlikely to run our simulation for longer 
# than that.
#
# We use the *same* seed that warnet is run with so that we can compare traffic. This is certainly 
# imperfect, but this is just an approximation anyway.
duration=$(( 7 * 24 * 60 * 60))
runtime=$((duration / 1000))
echo "Generating projected data for $duration seconds, will take: $runtime seconds with speedup of 1000"

for i in {1..10}; do
    echo "Data generation run: $i"
    sim-cli --clock-speedup 1000 --fix-seed 509064695903432291 -s "$simfile" -t "$duration"

    input_csv="results/htlc_forwards.csv"
    mv "$input_csv" "$projection_files/$i.csv"
done

cd ..

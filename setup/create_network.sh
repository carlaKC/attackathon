#!/bin/bash

set -e

usage() {
    echo "Usage: $0 <json_file_path> {duration}"
    echo "Example: $0 /path/to/file.json 100: creates network with 100 seconds of historical data"
    echo "Example: $0 /path/to/file.json: creates network but does not generate historical data"
    exit 1
}

if [ ! -d "warnet" ]; then
    echo "Error: Warnet directory not found. Make sure to clone Warnet before running this script."
    exit 1
fi

if [ ! -d "sim-ln" ]; then
    echo "Error: Sim-LN directory not found. Make sure to clone Sim-LN before running this script."
    exit 1
fi

if [ ! -d "circuitbreaker" ]; then
    echo "Error: Circuitbreaker directory not found. Make sure to clone circuitbreaker before running this script."
    exit 1
fi

if ! command -v rustc &> /dev/null; then
    echo "Error: Rust compiler (rustc) is not installed. Please install Rust from https://www.rust-lang.org/."
    exit 1
fi

# Check if required arguments are provided
if [ $# -gt 2 ]; then
    usage
fi

json_file="$1"
duration="$2"

current_directory=$(pwd)

# Check if JSON file exists
if [ ! -f "$json_file" ]; then
    echo "Error: JSON file '$json_file' not found."
    exit 1
fi

network_name=$(basename "$json_file" .json)
echo "Setting up network for $network_name"

sim_files="$current_directory"/attackathon/data/"$network_name"
echo "Creating simulation files in: "$sim_files""
mkdir -p $sim_files

docker_tag="carlakirkcohen/circuitbreaker:attackathon-$network_name"
raw_data="$sim_files/data.csv"

if [ -z "$2" ]; then
    echo "Duration argument not provided: not generating historical data for network"
else
    echo "Duration argument provided: generating historical data"
	
    simfile="$sim_files"/simln.json
    python3 attackathon/setup/lnd_to_simln.py "$json_file" "$simfile"
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

    # We want to generate two types of data here (TODO: fix seed):
    # 1. Bootstrapping data for the network, which provides historical reputations
    # 2. Projected revenue, which provides simulated network traffic without an attack present
    # Everything before ts_division is (1) and everything after is (2)
    sim_time=$(( duration + 14 * 24 * 60 * 60 ))
    ts_division=$(( ($(date +%s) + duration) * 1000000000 ))

    runtime=$((sim_time / 1000))
    echo "Generating historical and projected data for $sim_time seconds, will take: $runtime seconds with speedup of 1000"
    sim-cli --clock-speedup 1000 -s "$simfile" -t "$sim_time"

    # Once data is generated, we'll split it between historical bootstrap and projected revenue.
    input_csv="results/htlc_forwards.csv"

    # Filenames for output CSVs
    file_lt_division="historical_data.csv"
    file_ge_division="projected_forwards.csv"

    # Header for the output CSVs
    header=$(head -n 1 "$input_csv")

    # Creating new CSV files with headers
    echo "$header" > "$file_lt_division"
    echo "$header" > "$file_ge_division"

    # Processing each line in the input CSV, starting from the second line (skipping header)
    tail -n +2 "$input_csv" | while IFS=',' read -r incoming_amt incoming_expiry incoming_add_ts incoming_remove_ts \
                                    outgoing_amt outgoing_expiry outgoing_add_ts outgoing_remove_ts \
                                    forwarding_node forwarding_alias chan_in chan_out; do
    # Determine which file to write to based on condition
    if [ "$outgoing_remove_ts" -lt "$ts_division" ]; then
        echo "$incoming_amt,$incoming_expiry,$incoming_add_ts,$incoming_remove_ts,$outgoing_amt,$outgoing_expiry,$outgoing_add_ts,$outgoing_remove_ts,$forwarding_node,$forwarding_alias,$chan_in,$chan_out" >> "$file_lt_division"
    else
        echo "$incoming_amt,$incoming_expiry,$incoming_add_ts,$incoming_remove_ts,$outgoing_amt,$outgoing_expiry,$outgoing_add_ts,$outgoing_remove_ts,$forwarding_node,$forwarding_alias,$chan_in,$chan_out" >> "$file_ge_division"
    fi
    done

    # Copy bootstraped data into place for raw data to build circuitbreaker data.
    mv "$file_lt_division" "$raw_data"

    # Copy remaining data into folder for simulation so that analysis can use projected data for comparisons.
    mv "$file_ge_division" "$sim_files/projected.csv"
    cd ..
fi

echo "Building circuitbreaker image with new data"
cd circuitbreaker

if [[ -n $(git status --porcelain) ]]; then
    echo "Error: there are unsaved changes in circuitbreaker, please stash them!"
    exit 1
fi

if ! git remote | grep -q carla; then
    git remote add carla https://github.com/carlaKC/circuitbreaker
fi

git fetch carla > /dev/null 2>&1 || { echo "Failed to fetch carla/circuitbreaker"; exit 1; }
git checkout carla/attackathon > /dev/null 2>&1 || { echo "Failed to checkout carla/circuitbreaker/attackathon"; exit 1; }

cp "$raw_data" historical_data/raw_data_csv

# Build with no cache because docker is sometimes funny with not detecting changes in the files being copied in.
docker buildx build --platform linux/amd64,linux/arm64 -t "$docker_tag" --no-cache --push .

# Clean up everything we left in circuitbreaker.
git remote remove carla
git checkout master > /dev/null 2>&1
rm historical_data/raw_data_csv

cd ..

# Before we actually bump our timestamps, we'll spin up warnet to generate a graphml file that
# will use our generated data.
echo "Generating warnet file for network"
cd warnet 
pip install -e . > /dev/null 2>&1 

# Run warnet in the background and capture pid for shutdown.
warnet > /dev/null 2>&1 &
warnet_pid=$!

warnet_file="$sim_files"/"$network_name".graphml
warcli graph import-json "$json_file" --outfile="$warnet_file" --cb="$docker_tag" --ln_image=carlakirkcohen/lnd:attackathon> /dev/null 2>&1 

# Shut warnet down
kill $warnet_pid > /dev/null 2>&1
if ps -p $warnet_pid > /dev/null; then
    wait $warnet_pid 2>/dev/null
fi

cd ..

# We need to manually insert a sim-ln attribute + key to warnet graph.
data_tab='<key id="services" attr.name="services" attr.type="string" for="graph"/>'
escaped_data_tab=$(printf '%s\n' "$data_tab" | sed -e 's/[\/&]/\\&/g')

sed -i '' "  /<key id=\"target_policy\" for=\"edge\" attr.name=\"target_policy\" attr.type=\"string\" \/>/a\\
${escaped_data_tab}
" "$warnet_file"

simln_key='<data key="services">simln</data>'
escaped_simln_key=$(printf '%s\n' "$simln_key" | sed -e 's/[\/&]/\\&/g')
sed -i '' "/<graph edgedefault=\"directed\">/a\\
${escaped_simln_key}
" "$warnet_file"

echo "Setup complete!"

echo "Check in your data files and tell participants to run with network name: $network_name"

#!/bin/bash

set -e

usage() {
    echo "Usage: $0 <json_file_path> {duration}"
    echo "Example: $0 /path/to/file.json 100: creates network with 100 seconds of historical data"
    echo "Example: $0 /path/to/file.json: creates network but does not generate historical data"
    exit 1
}

if [ "$(basename "$PWD")" != "attackathon" ]; then
  echo "Script must be run from inside the attackathon repo."
  exit 1
fi

git submodule update --init  --recursive

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

if [ ! -d "lrc" ]; then
    echo "Error: LRC directory not found. Please fix submodules."
    exit 1
fi

if ! command -v rustc &> /dev/null; then
    echo "Error: Rust compiler (rustc) is not installed. Please install Rust from https://www.rust-lang.org/."
    exit 1
fi

if ! command -v go &> /dev/null; then
	echo "Error: Golang is not installed."
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

sim_files="$current_directory"/data/"$network_name"
echo "Creating simulation files in: "$sim_files""
mkdir -p $sim_files

docker_tag="carlakirkcohen/circuitbreaker:attackathon-$network_name"
raw_data="$sim_files/data.csv"

if [ -z "$2" ]; then
    echo "Duration argument not provided: not generating historical data for network"
else
    echo "Duration argument provided: generating historical data"

    simfile="$sim_files"/simln.json
    python3 setup/lnd_to_simln.py "$json_file" "$simfile"
    cd sim-ln

    if [[ -n $(git status --porcelain) ]]; then
        echo "Error: there are unsaved changes in sim-ln, please stash them!"
        exit 1
    fi

    echo "Installing sim-ln for data generation"
    cargo install --locked --path sim-cli

    # First, generate data for the duration specified to bootstrap forwarding history for the nodes.
    # We fix this seed, but not with the value that we'll be running warnet with (because this is history).
    runtime=$((duration / 1000))
    echo "Generating historical data for $duration seconds, will take: $runtime seconds with speedup of 1000"
    sim-cli --clock-speedup 1000 --fix-seed 13995354354227336701 -s "$simfile" -t "$duration" -d "$sim_files"

    input_csv="$sim_files/results/htlc_forwards.csv"
    mv "$input_csv" "$raw_data"
	rm -rf "$sim_files/results"

    cd ..
fi

# Generate reputation scores from raw sim ln data
echo "Generating reputation data from raw data"
cd lrc
go run cmd/main.go "$sim_files"

reputation_data="$sim_files/reputations.csv"
mv "${network_name}_reputations.csv" "$reputation_data"

cd ..

echo "Building circuitbreaker image with new data"
cd circuitbreaker

if [[ -n $(git status --porcelain) ]]; then
    echo "Error: there are unsaved changes in circuitbreaker, please stash them!"
    exit 1
fi

cp "$reputation_data" historical_data/raw_data_csv

# Build with no cache because docker is sometimes funny with not detecting changes in the files being copied in.
docker buildx build --platform linux/amd64,linux/arm64 -t "$docker_tag" --no-cache --push .

# Clean up everything we left in circuitbreaker.
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

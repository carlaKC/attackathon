#!/bin/bash

# Function to check if a command is available
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "Error: $1 is not installed."
        exit 1
    fi
}

check_command just
check_command docker

if [ ! -d "warnet" ]; then
    git clone https://github.com/bitcoin-dev-project/warnet
fi

cd warnet

if [ -n "$(git status --porcelain)" ]; then
    echo "There are uncommitted changes in warnet, please stash them!"
    exit 1
fi

# Grab attackathon branch that will run with a fixed seed.
git remote add carla https://github.com/carlaKC/warnet
git fetch carla > /dev/null 2>&1 || { echo "Failed to fetch carla/warnet"; exit 1; }
git checkout carla/attackathon > /dev/null 2>&1 || { echo "Failed to checkout carla/warnet/attackathon"; exit 1; }
git remote remove carla

# Check whether running docker desktop or minikube.
docker_info=$(docker info)
if grep -q "Operating System:.*Desktop" <<< "$docker_info"; then
    docker_desktop=true
else
    docker_desktop=false
fi

# Only ask this question once, otherwise it's annoying.
if [ "$docker_desktop" = true ]; then
    echo "Detected docker desktop running."
else
    echo "Detected minikube running."
fi

read -p "Is this correct (y/n): " confirm
if [ "$confirm" != "y" ]; then
    echo "Unable to detect kubernetes platform - please open an issue with the output of $ docker info"
    exit 1
fi

# Check Docker info and start accordingly
if [ "$docker_desktop" = true ]; then
    echo "Starting warnet for Docker Desktop."
    just startd
else
    echo "Starting warnet for Minikube."
    just start
fi

# Port forward for warcli
echo "Port forwarding from kubernetes to warnet cluster for warcli (don't close this!)"
just p

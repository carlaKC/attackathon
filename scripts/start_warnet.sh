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

if [ "$(basename "$PWD")" != "attackathon" ]; then
  echo "Script must be run from inside the attackathon repo."
  exit 1
fi

# Update submodules so that all code is checked out.
git submodule update --init  --recursive

if [ ! -d "warnet" ]; then
	echo "Warnet is not checked out!"
fi

cd warnet

current_branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$current_branch" != "attackathon" ]; then
  echo "Expected to be on attackathon branch, got: '$current_branch'."
  exit 1
fi

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

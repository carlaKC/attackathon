#!/bin/bash

if [ "$(basename "$PWD")" != "attackathon" ]; then
  echo "Script must be run from inside the attackathon repo."
  exit 1
fi

if [ ! -d "warnet" ]; then
    echo "Error: warnet directory not found."
	exit 1
fi

cd warnet

docker_info=$(docker info)

# Setup depends on docker or docker desktop.
if grep -q "Operating System:.*Desktop" <<< "$docker_info"; then
    echo "Stopping warnet for docker desktop."
    just stopd
else
    echo "Stopping warnet for minikube."
    just stop
fi

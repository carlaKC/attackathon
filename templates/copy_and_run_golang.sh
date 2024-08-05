#!/bin/bash

echo "Removing old golang code in container"
kubectl exec -it warnet-armada/flagship -- rm -rf /golang

echo "Copying code in /golang into container"
kubectl cp golang warnet-armada/flagship:/

if ! kubectl exec -it flagship -n warnet-armada -- bash -c 'cat run.sh | grep -q "go install"'; then
    echo "Adding instructions to run script"

    kubectl exec -it flagship -n warnet-armada -- bash -c 'echo "export PEER=44" >> /run.sh'
    kubectl exec -it flagship -n warnet-armada -- bash -c 'echo "export RESOURCES=SLOTS" >> /run.sh'
    kubectl exec -it flagship -n warnet-armada -- bash -c 'echo "export SPEED=FAST" >> /run.sh'
    kubectl exec -it flagship -n warnet-armada -- bash -c 'echo "cd /golang" >> /run.sh'
    kubectl exec -it flagship -n warnet-armada -- bash -c 'echo "go install" >> /run.sh'
    kubectl exec -it flagship -n warnet-armada -- bash -c 'echo "/root/go/bin/attackathon" >> /run.sh'
fi

echo "Running script 😈"
kubectl exec -it flagship -n warnet-armada -- ./run.sh

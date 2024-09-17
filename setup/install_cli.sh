#!/bin/bash

# Grab bitcoin/lnd based on arch.
ARCH=$(dpkg --print-architecture)

if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    echo "Installing bitcoin / lnd for $ARCH"
    BITCOIN_URL="https://bitcoincore.org/bin/bitcoin-core-26.1/bitcoin-26.1-aarch64-linux-gnu.tar.gz"
elif [ "$ARCH" = "amd64" ]; then
    echo "Installing bitcoin / lnd for amd64"
    BITCOIN_URL="https://bitcoincore.org/bin/bitcoin-core-26.1/bitcoin-26.1-x86_64-linux-gnu.tar.gz"
else
    echo "Unsupported architecture $ARCH"
    exit 1
fi

# Bitcoin unzips without the arch suffix.
bitcoin_dir=bitcoin-26.1

# Download Bitcoin Core
wget "$BITCOIN_URL"
tar -xvf "$(basename "$BITCOIN_URL")"
mv "$bitcoin_dir/bin/bitcoin-cli" /bin

# Download LND and install it on custom branch so that endorsement signals are available on lncli.
git clone https://github.com/carlaKC/lnd 
cd lnd
git checkout attackathon
make release-install
mv /root/go/bin/lncli /bin
cd .. 

# Clean up downloaded files: both the unpacked dirs and the targz.
rm -rf "$bitcoin_dir" "$(basename "$BITCOIN_URL")" lnd

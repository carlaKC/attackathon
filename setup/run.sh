#!/bin/bash

# Check if the 'credentials' directory does not exist
if [ ! -d "credentials" ]; then
    echo "LND credentials not found in image"
	exit 1
fi

# LND credentials are exported as environment variables for 
# your program to use.
export LND_0_RPCSERVER="lightning-0.warnet-armada"
export LND_0_CERT="/credentials/lnd0-tls.cert"
export LND_0_MACAROON="/credentials/lnd0-admin.macaroon"

export LND_1_RPCSERVER="lightning-1.warnet-armada"
export LND_1_CERT="/credentials/lnd1-tls.cert"
export LND_1_MACAROON="/credentials/lnd1-admin.macaroon"

export LND_2_RPCSERVER="lightning-2.warnet-armada"
export LND_2_CERT="/credentials/lnd2-tls.cert"
export LND_2_MACAROON="/credentials/lnd2-admin.macaroon"

export TARGET=$(cat target.txt)

# Fill in code here to:
# - Clone your repo
# - Install your program
# - Run it with the certs/macaroons provided above

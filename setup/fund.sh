#!/bin/bash

# Create a wallet so that we can use generate elsewhere, even though we're not using it here.
bitcoin-cli createwallet test

lncli0="lncli --network=regtest --tlscertpath=/credentials/lnd0-tls.cert --macaroonpath=/credentials/lnd0-admin.macaroon --rpcserver=lightning-0.warnet-armada "
lncli1="lncli --network=regtest --tlscertpath=/credentials/lnd1-tls.cert --macaroonpath=/credentials/lnd1-admin.macaroon --rpcserver=lightning-1.warnet-armada "
lncli2="lncli --network=regtest --tlscertpath=/credentials/lnd2-tls.cert --macaroonpath=/credentials/lnd2-admin.macaroon --rpcserver=lightning-2.warnet-armada "

lnd_0_get_addr=$($lncli0 newaddress p2tr)
lnd_0_addr=$(echo "$lnd_0_get_addr" | jq -r '.address')

lnd_1_get_addr=$($lncli1 newaddress p2tr)
lnd_1_addr=$(echo "$lnd_1_get_addr" | jq -r '.address')

lnd_2_get_addr=$($lncli2 newaddress p2tr)
lnd_2_addr=$(echo "$lnd_2_get_addr" | jq -r '.address')

echo "Funding LND nodes"
bitcoin-cli generatetoaddress 20 "$lnd_0_addr"
bitcoin-cli generatetoaddress 20 "$lnd_1_addr"
bitcoin-cli generatetoaddress 20 "$lnd_2_addr"

echo "Mining to confirm coinbase"
bitcoin-cli generatetoaddress 100 "$lnd_0_addr"

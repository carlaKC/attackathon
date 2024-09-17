#!/bin/bash

tmp_dir=$(mktemp -d)
echo $tmp_dir
kubectl cp warnet-armada/lnd0:root/.lnd/tls.cert $tmp_dir/lnd0-tls.cert
kubectl cp warnet-armada/lnd1:root/.lnd/tls.cert $tmp_dir/lnd1-tls.cert
kubectl cp warnet-armada/lnd2:root/.lnd/tls.cert $tmp_dir/lnd2-tls.cert
kubectl cp warnet-armada/lnd0:root/.lnd/data/chain/bitcoin/regtest/admin.macaroon $tmp_dir/lnd0-admin.macaroon
kubectl cp warnet-armada/lnd1:root/.lnd/data/chain/bitcoin/regtest/admin.macaroon $tmp_dir/lnd1-admin.macaroon
kubectl cp warnet-armada/lnd2:root/.lnd/data/chain/bitcoin/regtest/admin.macaroon $tmp_dir/lnd2-admin.macaroon

# In addition to our credentials, we'll create a list of pubkeys/ips to connect to. 
json_data=$(warcli lncli 0 describegraph)
pub_keys=$(echo "$json_data" | jq -r '.nodes[] | select(.addresses != null and .addresses != []) | .pub_key')
addrs=$(echo "$json_data" | jq -r '.nodes[] | select(.addresses != null and .addresses != []) | .addresses[].addr')

# Loop through each entry and print pub_key and addr
for i in $(seq 1 $(($(echo "$pub_keys" | wc -l)-1))); do
    echo "$(echo "$pub_keys" | sed -n ${i}p)@$(echo "$addrs" | sed -n ${i}p)" >> "$tmp_dir"/nodes.txt
done

kubectl cp $tmp_dir warnet-armada/flagship:/credentials

import json
import sys

def convert_to_sim_network(input_file, output_file):
    with open(input_file, 'r') as f:
        data = json.load(f)

    nodes = data.get('nodes', [])
    node_pubkey_index = {node['pub_key']: index for index, node in enumerate(nodes)}

    sim_network = []

    scid_block = 300
   
    # Sort edges by channel_id to mimic the output of LND's describegraph.
    sorted_edges = sorted(data["edges"], key=lambda chan: int(chan['channel_id']))

    for index, edge in enumerate(sorted_edges):
        node_1_policy = edge.get('node1_policy', None)
        node_2_policy = edge.get('node2_policy', None)

        if not node_1_policy or not node_2_policy:
            print(f"Warning: Skipping edge with channel ID {edge['channel_id']} because node1 or node2 policy is null.")
            continue

        # Capacity is expressed in sats.
        capacity_msat = int(edge['capacity']) * 1000

        # Calculate short channel ID, based on warnet indexing
        scid_tx_index = 1
        scid_output_index = 0

        scid = (scid_block << 40) | (scid_tx_index << 16) | scid_output_index

        node_1_pubkey = edge['node1_pub']
        node_2_pubkey = edge['node2_pub']

        node_1_alias = str(node_pubkey_index.get(node_1_pubkey))
        node_2_alias = str(node_pubkey_index.get(node_2_pubkey))

        node_1 = {
            "pubkey": node_1_pubkey,
            "alias": node_1_alias,
            "max_htlc_count": 483,
            "max_in_flight_msat": capacity_msat,
            "min_htlc_size_msat": int(node_1_policy['min_htlc']),
            "max_htlc_size_msat": int(node_1_policy['max_htlc_msat']),
            "cltv_expiry_delta": int(node_1_policy['time_lock_delta']),
            "base_fee": int(node_1_policy['fee_base_msat']),
            "fee_rate_prop": int(node_1_policy['fee_rate_milli_msat'])
        }

        node_2 = {
            "pubkey": node_2_pubkey,
            "alias": node_2_alias,
            "max_htlc_count": 15,
            "max_in_flight_msat": capacity_msat,
            "min_htlc_size_msat": int(node_2_policy['min_htlc']),
            "max_htlc_size_msat": int(node_2_policy['max_htlc_msat']),
            "cltv_expiry_delta": int(node_2_policy['time_lock_delta']),
            "base_fee": int(node_2_policy['fee_base_msat']),
            "fee_rate_prop": int(node_2_policy['fee_rate_milli_msat'])
        }

        sim_network.append({
            "scid": scid,
            "capacity_msat": capacity_msat,
            "node_1": node_1,
            "node_2": node_2
        })

        # Add one to scid block height, as we create one channel per block.
        scid_block += 1

    output_data = {"sim_network": sim_network}

    with open(output_file, 'w') as f:
        json.dump(output_data, f, indent=2)

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python script.py input_file output_file")
        sys.exit(1)

    input_file = sys.argv[1]
    output_file = sys.argv[2]

    convert_to_sim_network(input_file, output_file)

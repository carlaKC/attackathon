import csv
import json
import os
import sys

def parse_channel_info(json_data):
    channel_map = {}

    for channel in json_data["sim_network"]:
        scid = str(channel["scid"])  # Ensure scid is used as a string for matching
        node_1_alias = channel["node_1"]["alias"]
        node_1_base_fee = channel["node_1"]["base_fee"]
        node_2_alias = channel["node_2"]["alias"]
        node_2_base_fee = channel["node_2"]["base_fee"]

        # Store channel info in the map
        channel_map[scid] = {
            node_1_alias: node_1_base_fee,
            node_2_alias: node_2_base_fee,
        }

    return channel_map

def get_channel_revenue(file_path, revenue_period_ns=7 * 24 * 60 * 60 * 1e9):
    revenue_by_channel = {}
    timestamp_limit = None

    with open(file_path, 'r') as file:
        reader = csv.DictReader(file)
        for row in reader:
            incoming_amt = int(row['incoming_amt'])
            outgoing_amt = int(row['outgoing_amt'])
            chan_out = row['chan_out'].strip()  # Strip whitespace and ensure it matches JSON format
            forwarding_alias = row['forwarding_alias'].strip()  # Strip whitespace
            incoming_add_ts = int(row['incoming_add_ts'])
            incoming_remove_ts = int(row['incoming_remove_ts'])

            # Determine the timestamp limit based on the first entry's timestamp.
            if timestamp_limit is None:
                timestamp_limit = incoming_add_ts + revenue_period_ns

            # Break the loop if the incoming timestamp exceeds the period.
            if incoming_add_ts >= timestamp_limit:
                break

            # Accumulate revenue and track forwarding_alias for each distinct chan_out within the period.
            if incoming_remove_ts < timestamp_limit:
                if chan_out not in revenue_by_channel:
                    revenue_by_channel[chan_out] = {"alias": forwarding_alias, "revenue": 0}
                
                # Assuming the forwarding_alias is consistent for a chan_out
                revenue_by_channel[chan_out]["revenue"] += (incoming_amt - outgoing_amt)

    return revenue_by_channel

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python unconditional_vs_revenue.py <network_name>") 
        sys.exit(1)

    attack_duration = 14 * 24 * 60 * 60 * 1e6

    network_name = sys.argv[1]
    network_path = os.path.join("data", network_name)

    revenue_file = os.path.join(network_path, "data.csv")
    revenue_per_channel = get_channel_revenue(revenue_file, attack_duration)

    sim_ln_file = os.path.join(network_path, "simln.json")
    with open(sim_ln_file, 'r') as file:
        network_channels = json.load(file)

    chan_map = parse_channel_info(network_channels)

    for chan_out, revenue_info in revenue_per_channel.items():
        forwarding_alias = revenue_info['alias']
        base_fee = chan_map.get(chan_out, {}).get(forwarding_alias, "N/A")
        uncond = attack_duration / 90 * 242 * base_fee * 0.01
        revenue = revenue_info['revenue']

        if base_fee == 0:
            continue

        if revenue > uncond: 
            print(f"Revenue: {revenue} not covered by unconditional unconditional: {uncond}")

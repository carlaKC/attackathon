import sys
import os
import json

def candidate_nodes(source_data, target):
    # Assuming source_data is a list or dictionary containing nodes
    if isinstance(source_data, dict):
        nodes = source_data.get('nodes', [])  # Adjust key based on your JSON structure
    elif isinstance(source_data, list):
        nodes = source_data
    else:
        nodes = []
    node_capacity = {node['pub_key']: 0 for node in nodes}

    # Set to store nodes that are connected to the target
    connected_to_target = set()

    # Calculate total capacities and find nodes connected to the target
    for edge in source_data.get('edges'):
        node1 = edge['node1_pub']
        node2 = edge['node2_pub']
        capacity = int(edge['capacity'])

        # Increment capacity for both nodes
        node_capacity[node1] += capacity
        node_capacity[node2] += capacity

        # Check if either node is connected to the target
        if target in (node1, node2):
            connected_to_target.add(node1)
            connected_to_target.add(node2)

    # Create a list of node pubkeys not connected to the target
    filtered_nodes = [node for node in node_capacity if node not in connected_to_target]

    # Sort the nodes by their total edge capacity in descending order
    sorted_nodes = sorted(filtered_nodes, key=lambda node: node_capacity[node], reverse=True)

    return sorted_nodes

if __name__ == "__main__":
    if len(sys.argv) != 4:
        print("Usage: python add_attacking_node.py source_network_name target_pubkey output_network")
        sys.exit(1)

    source_network_name = sys.argv[1]
    current_directory = os.getcwd()
    sim_files = os.path.join(current_directory, "attackathon", "data", source_network_name)
    source_graph = os.path.join(sim_files, f"{source_network_name}.json")

    # Get target node for the reference network.
    source_target = sys.argv[2]

    # Get candidate nodes that aren't connected to our target.
    with open(source_graph, 'r') as f:
        source_data = json.load(f)

    # Get total count of nodes in the network, and nodes that aren't connected to target.
    node_count = len(source_data.get('nodes'))
    candidate_nodes = candidate_nodes(source_data, source_target)

    # Create 10% of the channels in the network for the attacker.
    channel_count = int(node_count / 10)
    print(f"Graph size: {node_count}, with {len(candidate_nodes)} candidates not connected to target {source_target}, adding {channel_count}.")

    # Insert the target node into the json, these values don't matter because we replace them, we just need a unique pubkey.
    attacker_pubkey = "035a43121d24b2ff465e85af9c07963701f259b5ce4ee636e3aeb503cc64142c11"
    new_node = {
        "pub_key": attacker_pubkey,
        "alias": "attacker"
    }

    nodes = source_data.get('nodes', [])
    nodes.append(new_node)
    source_data['nodes'] = nodes

    channel_capacity = 10000000
    max_htlc_msat = (channel_capacity - 1000) *1000

    # Create a base edge where node 1 has zero fees (the attacker).
    base_edge = {
        "node1_pub": "",
        "node2_pub": "",
        "channel_id": "912080079760850944",
        "capacity": channel_capacity,
        "node1_policy": {
            "time_lock_delta": 40,
            "min_htlc": "1",
            "fee_base_msat": "0",
            "fee_rate_milli_msat": "0",
            "max_htlc_msat": max_htlc_msat,
        },
        "node2_policy": {
            "time_lock_delta": 144,
            "min_htlc": "1",
            "fee_base_msat": "1000",
            "fee_rate_milli_msat": "1000",
            "max_htlc_msat": max_htlc_msat,
        },
    }
    
    edges = source_data.get('edges', [])
    for i in range(channel_count):
        node = nodes[i]
        edge = base_edge.copy()

        # Set node 1 to the attacker and node 2 to the candidate node.
        edge['node1_pub'] = attacker_pubkey
        edge['node2_pub'] = node['pub_key']
        edge['channel_id'] = i + 1

        # Append the new edge to the edges list
        edges.append(edge)

    # Now add a single channel between the target node and the attacker.
    edge = base_edge.copy()

    # Set node 1 to the attacker and node 2 to the candidate node.
    edge['node1_pub'] = attacker_pubkey
    edge['node2_pub'] = source_target
    edge['channel_id'] = 999
    edge['capacity'] = channel_capacity * channel_count

    # Append the new edge to the edges list
    edges.append(edge)

    # Update the data with new edges
    source_data['edges'] = edges

    # Save modified graph
    dest_network_name = sys.argv[3]
    sim_files = os.path.join(current_directory, "attackathon", "data", dest_network_name)
    dest_graph = os.path.join(sim_files, f"{dest_network_name}.json")
   
    # Save the modified JSON data back to the file
    with open(dest_graph, 'w') as f:
        json.dump(source_data, f, indent=4)

import sys
import os
import analyse_attack

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python pull_data.py <node_count>")
        sys.exit(1)

    node_count = int(sys.argv[1])
    directory = f"ln_{node_count}"

    os.makedirs(directory, exist_ok=True)

    for index in range(node_count):
        print(f"Pulling node {index}")
        padded_node_id = str(index).zfill(6)

        fwd_file = os.path.join(directory, f"{padded_node_id}_forwarding_history.json")
        analyse_attack.save_forwarding_history(padded_node_id, fwd_file)

        threshold_file = os.path.join(directory, f"{padded_node_id}_thresholds.json")
        analyse_attack.save_thresholds(padded_node_id, threshold_file)

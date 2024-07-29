import json
import sys
from collections import defaultdict

def calculate_slot_jamming(local_amt, base_fee_msat, fee_rate_ppm):
    return base_fee_msat < (local_amt / 242 - 1) * (fee_rate_ppm / 1e6)

def parse_json_file(filename):
    with open(filename, 'r') as file:
        data = json.load(file)
    return data['edges']

def main(filename):
    edges = parse_json_file(filename)
    counts = defaultdict(int)

    for edge in edges:
        local_amt = int(edge['capacity'])

        node1_policy = edge.get('node1_policy')
        node2_policy = edge.get('node2_policy')

        if node1_policy:
            source_base_fee_msat = int(node1_policy.get('fee_base_msat', 0))
            source_fee_rate_ppm = int(node1_policy.get('fee_rate_milli_msat', 0))
            source_slot_jamming = calculate_slot_jamming(local_amt, source_base_fee_msat, source_fee_rate_ppm)
            counts[source_slot_jamming] += 1

        if node2_policy:
            target_base_fee_msat = int(node2_policy.get('fee_base_msat', 0))
            target_fee_rate_ppm = int(node2_policy.get('fee_rate_milli_msat', 0))
            target_slot_jamming = calculate_slot_jamming(local_amt, target_base_fee_msat, target_fee_rate_ppm)
            counts[target_slot_jamming] += 1
    
    print(f"Should slot jam: {counts[True]}")
    print(f"Should liquidity jam: {counts[False]}")

if __name__ == "__main__":
    main(sys.argv[1])

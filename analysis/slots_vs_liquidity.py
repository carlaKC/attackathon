import json
import sys
from collections import defaultdict

def should_slot_jam(local_amt, base_fee_msat, fee_rate_ppm, max_htlc, min_htlc):
    fast_jam_rounds = 10
    height_delta = 160
    resolution_period = 90
    uncond_rate = 0.01

    liquidity_htlc_fee = max_htlc * fee_rate_ppm/1e6 + base_fee_msat
    liquidity_prepay = liquidity_htlc_fee * height_delta*60*10/resolution_period * local_amt/max_htlc
    liquidity_uncond = liquidity_htlc_fee * uncond_rate * local_amt/max_htlc * fast_jam_rounds

    slot_htlc_fee = min_htlc * fee_rate_ppm/1e6 + base_fee_msat
    slot_prepay = slot_htlc_fee * height_delta*60*10/resolution_period * 242
    slot_uncond = slot_htlc_fee * uncond_rate * 242 * fast_jam_rounds

    #print(f"Capacity: {local_amt} with base: {base_fee_msat} and rate: {fee_rate_ppm} ({min_htlc} / {max_htlc})")
    #print(f"Liquidity: prepay: {liquidity_prepay} uncond: {liquidity_uncond}")
    #print(f"Slots: prepay: {slot_prepay} uncond: {slot_uncond}\n")

    return liquidity_prepay + liquidity_uncond > slot_prepay + slot_uncond

def parse_json_file(filename):
    with open(filename, 'r') as file:
        data = json.load(file)
    return data['edges']

def main(filename):
    edges = parse_json_file(filename)
    counts = defaultdict(int)

    for edge in edges:
        # Convert sat to msat
        local_amt = int(edge['capacity']) * 1000

        node1_policy = edge.get('node1_policy')
        node2_policy = edge.get('node2_policy')

        if node1_policy:
            source_base_fee_msat = int(node1_policy.get('fee_base_msat', 1000))
            source_fee_rate_ppm = int(node1_policy.get('fee_rate_milli_msat', 1))
            source_max_htlc = int(node1_policy.get('max_htlc_msat', local_amt))
            source_min_htlc = int(node1_policy.get('min_hltc', 1))

            source_slot_jamming = should_slot_jam(local_amt, source_base_fee_msat, source_fee_rate_ppm, source_max_htlc, source_min_htlc)
            counts[source_slot_jamming] += 1

        if node2_policy:
            target_base_fee_msat = int(node2_policy.get('fee_base_msat', 0))
            target_fee_rate_ppm = int(node2_policy.get('fee_rate_milli_msat', 0))
            target_max_htlc = int(node2_policy.get('max_htlc_msat', local_amt))
            target_min_htlc = int(node2_policy.get('min_hltc', 1))

            target_slot_jamming = should_slot_jam(local_amt, target_base_fee_msat, target_fee_rate_ppm, target_max_htlc, target_min_htlc)
            counts[target_slot_jamming] += 1
    
    print(f"Should slot jam: {counts[True]}")
    print(f"Should liquidity jam: {counts[False]}")

if __name__ == "__main__":
    main(sys.argv[1])

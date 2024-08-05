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

def change_in_uncond_fee(local_amt, base_fee_msat, fee_rate_ppm, max_htlc, min_htlc, base_premium_rate):
    fast_jam_rounds = 10
    height_delta = 160
    resolution_period = 90
    uncond_rate = 0.01

    liquidity_htlc_fee = max_htlc * fee_rate_ppm / 1e6 + base_fee_msat
    liquidity_prepay = liquidity_htlc_fee * height_delta * 60 * 10 / resolution_period * local_amt / max_htlc
    liquidity_uncond = liquidity_htlc_fee * uncond_rate * local_amt / max_htlc * fast_jam_rounds

    slot_htlc_fee = min_htlc * fee_rate_ppm / 1e6 + base_fee_msat
    slot_prepay = slot_htlc_fee * height_delta * 60 * 10 / resolution_period * 242
    slot_uncond = slot_htlc_fee * uncond_rate * 242 * fast_jam_rounds

    base_premium = base_fee_msat * base_premium_rate
    minimum_uncond_old = slot_htlc_fee * 0.01
    minimum_uncond_new = minimum_uncond_old + base_premium

    maximum_uncond_old = liquidity_htlc_fee * 0.01
    maximum_uncond_new = maximum_uncond_old + base_premium

    average_size_msat = 44_700_000
    average_fee_msat = average_size_msat * fee_rate_ppm/1e6 + base_fee_msat
    average_uncond_old = average_fee_msat * 0.01
    average_uncond_new = average_uncond_old + base_premium
    
    if minimum_uncond_old ==0 or base_fee_msat == 0 and base_fee_msat == 2147483647:
        return 0,0,0,False
   
    return (
        minimum_uncond_new / slot_htlc_fee,
        maximum_uncond_new / liquidity_htlc_fee,
        average_uncond_new / average_fee_msat,
        True,
    )

def parse_json_file(filename):
    with open(filename, 'r') as file:
        data = json.load(file)
    return data['edges']

def main(filename):
    edges = parse_json_file(filename)
    counts = defaultdict(int)
    fee_base_msat_values = []

    min_increases = []
    max_increases = []
    avg_increases = []
    
    base_premium_rate = 0.1

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
            
            fee_base_msat_values.append(source_base_fee_msat)

            min_inc, max_inc, avg_inc, ok = change_in_uncond_fee(local_amt, source_base_fee_msat, source_fee_rate_ppm, source_max_htlc, source_min_htlc, base_premium_rate)
            if ok:
                min_increases.append(min_inc)
                max_increases.append(max_inc)
                avg_increases.append(avg_inc)

        if node2_policy:
            target_base_fee_msat = int(node2_policy.get('fee_base_msat', 0))
            target_fee_rate_ppm = int(node2_policy.get('fee_rate_milli_msat', 0))
            target_max_htlc = int(node2_policy.get('max_htlc_msat', local_amt))
            target_min_htlc = int(node2_policy.get('min_hltc', 1))

            target_slot_jamming = should_slot_jam(local_amt, target_base_fee_msat, target_fee_rate_ppm, target_max_htlc, target_min_htlc)
            counts[target_slot_jamming] += 1

            fee_base_msat_values.append(target_base_fee_msat)

            min_inc, max_inc, avg_inc, ok = change_in_uncond_fee(local_amt, target_base_fee_msat, target_fee_rate_ppm, target_max_htlc, target_min_htlc, base_premium_rate)
            if ok:
                min_increases.append(min_inc)
                max_increases.append(max_inc)
                avg_increases.append(avg_inc)

    print(f"Should slot jam: {counts[True]}")
    print(f"Should liquidity jam: {counts[False]}")

    
    fee_base_msat_values.sort()
    n = len(fee_base_msat_values)
    if n > 0:
        median_fee_base_msat = fee_base_msat_values[n // 2] if n % 2 == 1 else (fee_base_msat_values[n // 2 - 1] + fee_base_msat_values[n // 2]) / 2
    else:
        median_fee_base_msat = 0

    print(f"Median base fee: {median_fee_base_msat}")

   # Calculate averages
    avg_min_increase = sum(min_increases) / len(min_increases)
    avg_max_increase = sum(max_increases) / len(max_increases)
    avg_avg_increase = sum(avg_increases) / len(avg_increases)

    print(f"Average % of success for minimum htlc size: {avg_min_increase*100:.5}%")
    print(f"Average % of success for maximum htlc size: {avg_max_increase*100:.5}%")
    print(f"Average % of success fee for average payment: {avg_avg_increase*100:.5}%")

if __name__ == "__main__":
    main(sys.argv[1])

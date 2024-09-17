import costs
import re
import sys
import time
import json

def parse_summary(file_path):
    # Construct the full path to the summary.txt file
    summary_file_path = f"{file_path}/summary.txt"
    
    try:
        with open(summary_file_path, 'r') as file:
            # Read the first line of the file
            first_line = file.readline().strip()

            # Use a regular expression to find the numbers in the line
            match = re.search(r"Running analysis for period:\s+([\d.e+-]+)\s+->\s+([\d.e+-]+)", first_line)

            if match:
                # Extract the two numbers
                first_number = match.group(1)
                second_number = match.group(2)

                return float(first_number), int(second_number)
            else:
                print("No matching numbers found in the first line.")
                return None, None

    except FileNotFoundError:
        print(f"File not found: {summary_file_path}")
        return None, None

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python investigation results_dir")
        sys.exit(1)

    file_path = sys.argv[1]
    start_time_ns=1723466407000000000 
    end_time_ns=1723527651279804000   

    #start_time_ns, end_time_ns = parse_summary(file_path)
    #print(f"Got range: {start_time_ns} -> {end_time_ns} for dir: {file_path}")

    with open(f"{file_path}/lnd_0.json", 'r') as file:
        lnd_0_payments = json.load(file)
        # Just grab the first hop's pubkey as our target pubkey as we're always sending from lnd0 -> target
        target_pubkey = lnd_0_payments['payments'][0]['htlcs'][0]['route']['hops'][0]['pub_key']

    lnd_0 = costs.process_attacker_payments(lnd_0_payments['payments'], target_pubkey, start_time_ns, end_time_ns)
    
    with open(f"{file_path}/lnd_1.json", 'r') as file:
        lnd_1_payments = json.load(file)

    lnd_1 = costs.process_attacker_payments(lnd_1_payments['payments'], target_pubkey, start_time_ns, end_time_ns)

    total_payment_count = lnd_0['attacker_total'] + lnd_1['attacker_total']
    attacker_success_msat = lnd_0['attacker_success_msat'] + lnd_1['attacker_success_msat']
    attacker_unconditional_msat = lnd_0['attacker_unconditional_msat'] + lnd_1['attacker_unconditional_msat']

    attacker_to_target_success_msat = lnd_0['target_success_msat'] + lnd_1['target_success_msat']
    attacker_to_target_uncond_msat = lnd_0['target_unconditional_msat'] + lnd_1['target_unconditional_msat']

    attacker_total = attacker_success_msat + attacker_unconditional_msat
    
    fwd_file = f"{file_path}/forwarding_history.json"
    success_revenue, unconditional_revenue = costs.get_target_revenue(fwd_file, start_time_ns, end_time_ns)
    target_revenue = success_revenue + unconditional_revenue
    honest_revenue = target_revenue - attacker_to_target_uncond_msat - attacker_to_target_success_msat

    attacker_to_target_total = attacker_to_target_success_msat + attacker_to_target_uncond_msat
    attacker_to_target_percent = round(attacker_to_target_total * 100 / target_revenue, 2)
    honest_to_target_percent = round(honest_revenue * 100 / target_revenue, 2)

    print()
    print(f"Attacker sent: {total_payment_count} payments paying {attacker_total} msat fees")
    print(f"- Success fees: {attacker_success_msat} msat ({attacker_to_target_success_msat} to target)")
    print(f"- Unconditional fees: {attacker_unconditional_msat} msat ({attacker_to_target_uncond_msat} to target)\n")

    print(f"Target revenue under attack: {target_revenue} msat")
    print(f"- Success Fees: {success_revenue} msat")
    print(f"- Unconditional fees: {unconditional_revenue} msat\n")

    print("Breakdown of target's revenue under attack:")
    print(f"- Attacker paid {attacker_to_target_percent}%: {attacker_to_target_total} msat")
    print(f"- Honest traffic paid {honest_to_target_percent}%: {honest_revenue} msat\n")

    print(f"{attacker_success_msat},{attacker_unconditional_msat},{success_revenue},{unconditional_revenue},{attacker_to_target_total}")

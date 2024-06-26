import time
from datetime import datetime
import re
import subprocess
import csv
import json
import costs
import tempfile
import os
import sys
import target_jammed

lncli_commands = [
    "kubectl exec -it flagship -n warnet-armada -- lncli --network=regtest --tlscertpath=/credentials/lnd0-tls.cert --macaroonpath=/credentials/lnd0-admin.macaroon --rpcserver=lightning-0.warnet-armada",
    "kubectl exec -it flagship -n warnet-armada -- lncli --network=regtest --tlscertpath=/credentials/lnd1-tls.cert --macaroonpath=/credentials/lnd1-admin.macaroon --rpcserver=lightning-1.warnet-armada",
    "kubectl exec -it flagship -n warnet-armada -- lncli --network=regtest --tlscertpath=/credentials/lnd2-tls.cert --macaroonpath=/credentials/lnd2-admin.macaroon --rpcserver=lightning-2.warnet-armada"
]

def execute_command_and_save_output(command, output_file):
    with open(output_file, 'w') as f:
        subprocess.run(command, stdout=f, shell=True)

def save_forwarding_history(node_id):
    command = f"kubectl exec -it warnet-tank-ln-{node_id} -c ln-cb -- wget -qO- http://localhost:9235/api/forwarding_history"
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_file:
        temp_filename = temp_file.name
        execute_command_and_save_output(command, temp_filename)
        return temp_filename

def save_thresholds(node_id):
    command = f"kubectl exec -it warnet-tank-ln-{padded_node_id} -c ln-cb -- wget -qO- http://localhost:9235/api/reputation_thresholds"
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_file:
        temp_filename = temp_file.name
        execute_command_and_save_output(command, temp_filename)
        return temp_filename

def save_channel_list(node_id):
    command = f"warcli lncli {node_id} listchannels"
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_file:
        temp_filename = temp_file.name
        execute_command_and_save_output(command, temp_filename)
        return temp_filename

def get_pubkey(node_id):
    command = f"warcli lncli {node_id} getinfo"
    
    try:
        output = os.popen(command).read().strip()
        data = json.loads(output)
        identity_pubkey = data.get("identity_pubkey", None)
        
        return identity_pubkey

    except OSError as e:
        print(f"Error executing command: {e}")
        return None
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON output: {e}")
        return None

def execute_kubectl_cmd(command, pod):
    result = subprocess.run(['kubectl', command, 'pod', pod], stdout=subprocess.PIPE)
    return result.stdout.decode('utf-8')

def simln_start_time():
    pod_description = execute_kubectl_cmd('describe', 'warnet-simln')

    match = re.search(r'Started:\s+(.*)', pod_description)
    if match:
        started_time_str = match.group(1)
        dt = datetime.strptime(started_time_str, '%a, %d %b %Y %H:%M:%S %z')
        unix_timestamp = dt.timestamp()
        return int(unix_timestamp) * 1e9

    else:
        raise ValueError("Started time not found in the pod description: {pod_description}")

def get_projected_revenue(network_name, node_id, revenue_period_ns):
    file_path = os.path.join("data", network_name, "projected.csv")
    total_fees = 0
    timestamp_limit = None

    with open(file_path, 'r') as file:
        reader = csv.DictReader(file)
        for row in reader:
            incoming_amt = int(row['incoming_amt'])
            outgoing_amt = int(row['outgoing_amt'])
            forwarding_alias = row['forwarding_alias']
            incoming_add_ts = int(row['incoming_add_ts'])
            incoming_remove_ts = int(row['incoming_remove_ts'])
           
            # We want to only get entries for the period that we've defined to have a way to compare revenue to what we got in the simulation 
            # that ran for revenue_period_ns. We don't have a start time for this projected data, so we just grab our first timestamp as the 
            # start. This is imperfect, and may lead to us over-estimating revenue without an attack (especially if there was a long wait 
            # for the first payment to occur). This could possibly be improved by including the start time in the file name so we can get 
            # an exact start, but is okay for now.
            # 
            # We can't use actual timestamps here, because this data was generated once-off and has old timestamps (hasn't been "progressed"
            # to current times like we do for bootstrapped data, as this isn't actually necessary).
            if timestamp_limit is None:
                timestamp_limit = incoming_add_ts + revenue_period_ns

            if incoming_add_ts >= timestamp_limit:
                break
            
            if forwarding_alias == node_id and incoming_remove_ts < timestamp_limit:
                total_fees += (incoming_amt - outgoing_amt)
    
    return total_fees

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python analyse_attack.py <network_name> [attack_runtime_ns]")
        sys.exit(1)
    network_name = sys.argv[1]

    # If an end time was provided, use it. Otherwise we'll use the current time as an end time.
    if len(sys.argv) > 2:
        end_time = sys.argv[2]
    else:
        end_time = time.time_ns()

    # Use the time that sim-ln started as our start time, this is when payments would have started
    # to flow through the network.
    start_time = simln_start_time()
    print(f"Running analysis for period: {start_time} -> {end_time}")

    # Construct the file path
    file_path = os.path.join("data", network_name, "target.txt")

    if os.path.exists(file_path):
        with open(file_path, 'r') as file:
            node_id = file.read().strip()
            print(f"Running attack analysis for target node: {node_id}")
    else:
        print(f"The network at {file_path} does not exist.")
        sys.exit(1)

    padded_node_id = node_id.zfill(6)   
    forwarding_hist_file = save_forwarding_history(padded_node_id)
    channel_list_file = save_channel_list(padded_node_id)
    threshold_file = save_thresholds(padded_node_id)
    
    target_jammed.process_files(forwarding_hist_file, channel_list_file, threshold_file)

    target_pubkey = get_pubkey(node_id)

    total_payment_count = 0
    attacker_success_msat = 0
    attacker_unconditional_msat = 0

    target_total = 0
    attacker_to_target_success_msat = 0
    attacker_to_target_uncond_msat = 0

    for i, command in enumerate(lncli_commands):
        attacker_costs = costs.get_attacker_costs(command, target_pubkey)
        total_payment_count += attacker_costs['attacker_total']
        attacker_success_msat += attacker_costs['attacker_success_msat']
        attacker_unconditional_msat += attacker_costs['attacker_unconditional_msat']

        attacker_to_target_success_msat += attacker_costs['target_success_msat']
        attacker_to_target_uncond_msat += attacker_costs['target_unconditional_msat']

    attacker_total = attacker_success_msat + attacker_unconditional_msat
    print()
    print(f"Attacker sent: {total_payment_count} payments paying {attacker_total} msat fees")
    print(f"- Success fees: {attacker_success_msat} msat")
    print(f"- Unconditional fees: {attacker_unconditional_msat} msat\n")

    projected = get_projected_revenue(network_name, node_id, end_time - start_time)
    print(f"Target revenue without attack: {projected} msat")

    success_revenue, unconditional_revenue = costs.get_target_revenue(forwarding_hist_file)
    target_revenue = success_revenue + unconditional_revenue
    honest_revenue = target_revenue - attacker_to_target_uncond_msat - attacker_to_target_success_msat

    print(f"Target revenue under attack: {target_revenue} msat")
    print(f"- Success Fees: {success_revenue} msat")
    print(f"- Unconditional fees: {unconditional_revenue} msat\n")

    attacker_to_target_total = attacker_to_target_success_msat + attacker_to_target_uncond_msat
    attacker_to_target_percent = round(attacker_to_target_total * 100 / target_revenue, 2)
    honest_to_target_percent = round(honest_revenue * 100 / target_revenue, 2)
    print("Breakdown of target's revenue under attack:")
    print(f"- Attacker paid {attacker_to_target_percent}%: {attacker_to_target_total} msat")
    print(f"- Honest traffic paid {honest_to_target_percent}%: {honest_revenue} msat\n")


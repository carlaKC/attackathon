import time
import projected_revenue
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
import target_liquidity_jammed

lncli_commands = [
    "kubectl exec -it flagship -n warnet-armada -- lncli --network=regtest --tlscertpath=/credentials/lnd0-tls.cert --macaroonpath=/credentials/lnd0-admin.macaroon --rpcserver=lightning-0.warnet-armada",
    "kubectl exec -it flagship -n warnet-armada -- lncli --network=regtest --tlscertpath=/credentials/lnd1-tls.cert --macaroonpath=/credentials/lnd1-admin.macaroon --rpcserver=lightning-1.warnet-armada",
    "kubectl exec -it flagship -n warnet-armada -- lncli --network=regtest --tlscertpath=/credentials/lnd2-tls.cert --macaroonpath=/credentials/lnd2-admin.macaroon --rpcserver=lightning-2.warnet-armada"
]

def execute_command_and_save_output(command, filename):
    file_path = os.path.join(os.getcwd(), filename)
    with open(file_path, 'w') as file:
        subprocess.run(command, stdout=file, shell=True)

def save_forwarding_history(node_id, filename):
    command = f"kubectl -n warnet exec -it warnet-tank-ln-{node_id} -c ln-cb -- wget -qO- http://localhost:9235/api/forwarding_history"
    execute_command_and_save_output(command, filename)

def save_thresholds(node_id, filename):
    command = f"kubectl -n warnet exec -it warnet-tank-ln-{node_id} -c ln-cb -- wget -qO- http://localhost:9235/api/reputation_thresholds"
    execute_command_and_save_output(command, filename)

def save_channel_list(node_id, filename):
    command = f"warcli lncli {node_id} listchannels"
    execute_command_and_save_output(command, filename)

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
    result = subprocess.run(['kubectl', '-n', 'warnet', command, 'pod', pod], stdout=subprocess.PIPE)
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

    fwd_file = "forwarding_history.json"
    save_forwarding_history(padded_node_id, fwd_file)

    channel_file = "channels.json"
    save_channel_list(padded_node_id, "channels.json")

    threshold_file = "thresholds.json"
    save_thresholds(padded_node_id, threshold_file)
    
    target_pubkey = get_pubkey(node_id)

    total_payment_count = 0
    attacker_success_msat = 0
    attacker_unconditional_msat = 0

    target_total = 0
    attacker_to_target_success_msat = 0
    attacker_to_target_uncond_msat = 0

    for i, command in enumerate(lncli_commands):
        attacker_costs = costs.get_attacker_costs('lnd_'+str(i)+'.json', command, target_pubkey, start_time, end_time)
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

    mean_revenue, std_dev = projected_revenue.get_revenue_stats(network_name, node_id, end_time-start_time)
    print(f"Target revenue without attack: {mean_revenue} msat (standard deviation: {std_dev}")

    success_revenue, unconditional_revenue = costs.get_target_revenue(fwd_file, start_time, end_time)
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

    jam_time = target_jammed.get_jam_time(fwd_file, 440)
    print(f"Total amount slot jammed > 440 slots: {jam_time} minutes")

    liquidity_jam_time = target_liquidity_jammed.is_liquidity_jammed(channel_file, fwd_file, 0.9)
    print(f"Total amount liquidity jammed > 90%: {liquidity_jam_time} minutes\n")

    print("Result CSV:")
    print("attacker_success_msat,attacker_unconditional_msat,target_success_msat,target_unconditional_msat,attacker_to_target_msat,jam_time_min,liquidity_jam_time")
    print(f"{attacker_success_msat},{attacker_unconditional_msat},{success_revenue},{unconditional_revenue},{attacker_to_target_total},{jam_time},{liquidity_jam_time}")

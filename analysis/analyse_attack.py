import subprocess
import attacker_cost
import tempfile
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

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python script.py <node_id>")
        sys.exit(1)
    node_id = sys.argv[1]

    padded_node_id = node_id.zfill(6)   
    forwarding_hist_file = save_forwarding_history(padded_node_id)
    channel_list_file = save_channel_list(padded_node_id)
    threshold_file = save_thresholds(padded_node_id)
    
    target_jammed.process_files(forwarding_hist_file, channel_list_file, threshold_file)

    results = {}
    total_payment_count = 0
    total_success = 0
    total_upfront = 0

    for i, command in enumerate(lncli_commands):
        results[f'lncli{i}'] = attacker_cost.get_payment_cost(command)
        total_payment_count += results[f'lncli{i}']['payment_count']
        total_success += results[f'lncli{i}']['success']
        total_upfront += results[f'lncli{i}']['upfront']

    print(f"Attacker sent: {total_payment_count} payments paying {total_success+total_upfront} msat fees\n")

    for key, value in results.items():
        print(f"{key}: {value}")

    for i, command in enumerate(lncli_commands):
        attacker_cost.closed_channels_fees(command)

import subprocess
import json
import pandas as pd
import argparse

def run_lncli_command(command):
    result = subprocess.run(
        command.split(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE
    )
    
    if result.returncode != 0:
        print(f"Error running command {command}: {result.stderr.decode('utf-8')}")
        return None
    
    return json.loads(result.stdout.decode('utf-8'))

def get_revenue(node_id, max_entries=10):
    index_offset = 0
    total_fees = 0
    while True:
        response = run_lncli_command(f"warcli lncli {node_id} fwdinghistory --index_offset={index_offset} --max_events={max_entries}")

        if response is None:
            break
        
        forwarding_events = response.get('forwarding_events', [])
        for fwd in forwarding_events:
            total_fees += int(fwd.get('fee_msat'))

        num_forwards_returned = len(forwarding_events)
        
        if num_forwards_returned < max_entries:
            break
        
        # Prepare index_offset for the next call
        index_offset += max_entries
    
    return total_fees

def paginate_lncli_listpayments(command, max_payments_per_call=10):
    index_offset = 0
    all_payments = []
    
    while True:
        response = run_lncli_command(f"{command} listpayments --include_incomplete --paginate_forwards --index_offset={index_offset} --max_payments={max_payments_per_call}")

        if response is None:
            break
        
        payments = response.get('payments', [])
        all_payments.extend(payments)

        last_index_offset = response.get('last_index_offset', 0)
        num_payments_returned = len(payments)
        
        if num_payments_returned < max_payments_per_call:
            break
        
        # Prepare for the next call
        index_offset = last_index_offset
    
    return {
        'payments': all_payments,
    }


def process_attacker_payments(payments, target_pubkey):
    results = []

    attacker_total = 0
    attacker_success_msat = 0
    attacker_unconditional_msat = 0

    target_total = 0
    target_success_msat = 0
    target_unconditional_msat = 0

    for payment in payments:
        attacker_total +=1
        for htlc in payment["htlcs"]:
            success = htlc["status"] == "SUCCEEDED"
            
            route_fee = int(htlc["route"]["total_fees_msat"])
            attacker_unconditional_msat += route_fee
            
            if success:
                attacker_success_msat += route_fee

            for hop in htlc["route"]["hops"]:
                if hop["pub_key"] == target_pubkey:
                    target_total += 1
                    hop_fee_msat = int(hop["fee_msat"])

                    target_unconditional_msat += hop_fee_msat
                    if success:
                        target_success_msat += hop_fee_msat

    return {
            'attacker_total': attacker_total,
            'attacker_success_msat': attacker_success_msat,
            'attacker_unconditional_msat': attacker_unconditional_msat *0.01,
            'target_total': target_total,
            'target_success_msat': target_success_msat,
            'target_unconditional_msat': target_unconditional_msat * 0.001,
    }

def get_attacker_costs(command, target_pubkey, max_payments_per_call=10000):
    result = paginate_lncli_listpayments(command, max_payments_per_call)
    
    return process_attacker_payments(payments, target_pubkey)


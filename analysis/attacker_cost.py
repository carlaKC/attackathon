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

def extract_data(payment):
    data = {}
    if 'htlcs' in payment:
        for htlc in payment['htlcs']:
            status = htlc.get('status')
            total_fees_msat = htlc.get('route', {}).get('total_fees_msat')
            if status and total_fees_msat:
                return pd.Series({
                    'status': status,
                    'total_fees_msat': int(total_fees_msat)
                })
    return pd.Series({
        'status': None,
        'total_fees_msat': 0
    })

def get_channel_cost():
    # First make sure there's nothing open or pending, we don't have code for this.
    no_open_or_pending()

def closed_channels_fees():
    # Get closed channels information
    closed_channels = run_lncli_command(["lncli", "closedchannels"])

    # Iterate over each channel and print funding and closing transactions
    for channel in closed_channels['channels']:
        channel_point = channel['channel_point']
        funding_txid = channel_point.split(":")[0]
        closing_txid = channel['closing_tx_hash']

def no_open_or_pending():
    data = run_lncli_command('lncli listchannels')
    
    # Check if there are any channels
    if 'channels' in data and len(data['channels']) > 0:
        raise ValueError("Error: There are active channels.")
    else:
        print("No active channels.")

    # Check pending channels
    pending_channels_data = run_lncli_command('lncli pendingchannels')
    
    if (len(pending_channels_data.get('pending_open_channels', [])) > 0 or
        len(pending_channels_data.get('pending_closing_channels', [])) > 0 or
        len(pending_channels_data.get('pending_force_closing_channels', [])) > 0 or
        len(pending_channels_data.get('waiting_close_channels', [])) > 0):
        raise ValueError("Error: There are pending channels.")

def get_payment_cost(command, max_payments_per_call=10000):
    result = paginate_lncli_listpayments(command, max_payments_per_call)
    payment_count = len(result['payments'])
    
    if payment_count == 0:
        return {
           'payment_count': 0,
            'success': 0,
            'upfront': 0
        }

    payments_df = pd.DataFrame(result['payments'])
    payments_detail_df = payments_df.apply(extract_data, axis=1)
    
    success = payments_detail_df[payments_detail_df['status'] == 'SUCCEEDED']['total_fees_msat'].sum()
    upfront = payments_detail_df['total_fees_msat'].sum() * 0.01
    
    return {
        'payment_count': payment_count,
        'success': success,
        'upfront': upfront
    }

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('command', help='lncli command to be executed')
    parser.add_argument('--max_payments_per_call', type=int, default=10000, help='maximum number of payments per call')
    args = parser.parse_args()

    result = get_payment_cost(args.command, args.max_payments_per_call)
    print(json.dumps(result, indent=4))

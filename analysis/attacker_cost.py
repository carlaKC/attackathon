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

def main(command, max_payments_per_call=10000):
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

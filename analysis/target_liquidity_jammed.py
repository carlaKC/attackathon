import sys
import os
import pandas as pd
import json

def create_pair_schedule(forwarding_history_json_path):
    # Load forwarding_history.json
    with open(forwarding_history_json_path, 'r') as file:
        forwarding_history_data = json.load(file)

    # Extract the forwarding events
    forwards = forwarding_history_data.get('forwards', [])

    # Create the initial DataFrame
    rows = []
    for entry in forwards:
        if isinstance(entry, dict):
            rows.append({
                'addTimeNs': int(entry['addTimeNs']),
                'resolveTimeNs': int(entry.get('resolveTimeNs', 0)),
                'eventTimeNs': int(entry['addTimeNs']),
                'eventType': 'add',
                'incomingAmount': int(entry['incomingAmount']),
                'outgoingAmount': int(entry['outgoingAmount']),
                'shortChannelId_outgoing': int(entry['outgoingCircuit']['shortChannelId']),
                'shortChannelId_incoming': int(entry['incomingCircuit']['shortChannelId']),
            })
            if 'resolveTimeNs' in entry:
                rows.append({
                    'addTimeNs': int(entry['addTimeNs']),
                    'resolveTimeNs': int(entry['resolveTimeNs']),
                    'eventTimeNs': int(entry['resolveTimeNs']),
                    'eventType': 'resolve',
                    'incomingAmount': int(entry['incomingAmount']),
                    'outgoingAmount': int(entry['outgoingAmount']),
                    'shortChannelId_outgoing': int(entry['outgoingCircuit']['shortChannelId']),
                    'shortChannelId_incoming': int(entry['incomingCircuit']['shortChannelId']),
                })

    # Create a DataFrame from the rows
    pair_schedule_df = pd.DataFrame(rows)

    # Sort the DataFrame by eventTimeNs
    pair_schedule_df = pair_schedule_df.sort_values(by='eventTimeNs').reset_index(drop=True)

    return pair_schedule_df

def create_channels_df(channels_json_path):
    # Load channels.json
    with open(channels_json_path, 'r') as file:
        channels_data = json.load(file)

    # Create a DataFrame for channel capacities
    channels_df = pd.DataFrame(channels_data['channels'])
    channels_df['chan_id'] = channels_df['chan_id'].astype(int)
    channels_df['capacity'] = channels_df['capacity'].astype(int)*1000
    channels_df = channels_df[['chan_id', 'capacity']]

    return channels_df

def track_funds(pair_schedule_df, channels_df, jam_lim = 0.2):
    # Initialize a dictionary to track available funds
    #funds = {channel: {'incoming': cap // 2, 'outgoing': cap // 2} for channel, cap in zip(channels_df['chan_id'], channels_df['capacity'])}
    funds = {channel: {'incoming': 0, 'outgoing': 0} for channel, cap in zip(channels_df['chan_id'], channels_df['capacity'])}

    
    # List to store times when locked funds exceed 80%
    high_locked_times = []
    status = []
    for _, row in pair_schedule_df.iterrows():

        if row['eventType'] == 'add':
            if row['shortChannelId_incoming'] in funds:
                funds[row['shortChannelId_incoming']]['outgoing'] += row['incomingAmount']
            else:
                # Handle the missing key case, possibly initializing the entry 
                funds[row['shortChannelId_incoming']] = {'outgoing': row['incomingAmount'], 'incoming': 0}
            
            if row['shortChannelId_outgoing'] in funds:
                funds[row['shortChannelId_outgoing']]['incoming'] += row['outgoingAmount']
            else:
                # Handle the missing key case, possibly initializing the entry 
                funds[row['shortChannelId_outgoing']] = {'incoming': row['outgoingAmount'], 'outgoing': 0}

        elif row['eventType'] == 'resolve':
            funds[row['shortChannelId_incoming']]['outgoing'] -= row['incomingAmount']
            funds[row['shortChannelId_outgoing']]['incoming'] -= row['outgoingAmount']

        # Check if the locked amounts exceed 80% of available funds
        for channel, cap in zip(channels_df['chan_id'], channels_df['capacity']):
            status.append({
                    'time': row['eventTimeNs'],
                    'channel': channel,
                    'capacity': cap,
                    'incoming_locked': funds[channel]['incoming'],
                    'outgoing_locked': funds[channel]['outgoing']
                })
            if funds[channel]['incoming'] > jam_lim * (cap // 2):
                high_locked_times.append({
                    'time': row['eventTimeNs'],
                    'resolve_time': row['resolveTimeNs'],
                    'channel': channel,
                    'capacity': cap,
                    'direction': 'incoming',
                    'funds_locked': funds[channel]['incoming'],
                })
            if funds[channel]['outgoing'] > jam_lim * (cap // 2):
                high_locked_times.append({
                    'time': row['eventTimeNs'],
                    'resolve_time': row['resolveTimeNs'],
                    'channel': channel,
                    'capacity': cap,
                    'direction': 'outgoing',
                    'funds_locked': funds[channel]['outgoing']   
                })

    return pd.DataFrame(high_locked_times), pd.DataFrame(status)

def calculate_active_time(df):
    # Sort by start time
    df = df.sort_values(by='time')
    
    # Initialize variables
    total_active_time = 0
    current_start = None
    current_end = None
    
    for _, row in df.iterrows():
        start, end = row['time'], row['resolve_time']
        
        if current_start is None:
            current_start = start
            current_end = end
        else:
            if start <= current_end:
                # Overlapping period, extend the end time if needed
                current_end = max(current_end, end)
            else:
                # Non-overlapping period, add the previous period's duration
                total_active_time += current_end - current_start
                current_start = start
                current_end = end
    
    # Add the last period
    if current_start is not None:
        total_active_time += current_end - current_start
    
    return total_active_time

def is_liquidity_jammed(channels_file, forwarding_history_file, liq_jam_ratio=0.9):
    channels_df = create_channels_df(channels_file)
    pair_schedule_df = create_pair_schedule(forwarding_history_file)

    high_locked_df, st_df = track_funds(pair_schedule_df, channels_df, liq_jam_ratio)
    pair_schedule_df.head(10)

    grouped = high_locked_df.groupby(['channel', 'direction'])

    results = grouped.apply(calculate_active_time).reset_index()
    results.columns = ['channel', 'direction', 'total_active_time']
    results['total_active_time_minutes'] = results['total_active_time']/60000000000

    return results['total_active_time_minutes'].tolist()[0]

if __name__ == '__main__':
    if len(sys.argv) != 3:
        print("Usage: python script.py forwarding_history_file channels_file")
        sys.exit(1)

    forwarding_history_file = sys.argv[1]
    channels_file = sys.argv[2]

    ratio = 0.9
    jam_time = is_liquidity_jammed(channels_file, forwarding_history_file, ratio)
    print(f"Cumulative jam time occupied > 95% of liquidity : {jam_time}")

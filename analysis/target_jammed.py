import pandas as pd
import json
import sys

def was_channel_jammed(channel, eventDf):
    filtered_df = eventDf[(eventDf['shortChannelId_outgoing'] == channel) | 
                          (eventDf['shortChannelId_incoming'] == channel)]
    slot_count = 0
    time_slot_list = []
    for index, row in filtered_df.iterrows():
        if row['eventType'] == 'add':
            slot_count += 1
            time_slot_list.append({'time': row['eventTimeNs'], 'taken_slots': slot_count})
        elif row['eventType'] == 'resolve':
            slot_count -= 1
            time_slot_list.append({'time': row['eventTimeNs'], 'taken_slots': slot_count})
    result_df = pd.DataFrame(time_slot_list)
    return result_df

def process_all_channels(pair_schedule_df):
    unique_channels = pd.unique(
        pair_schedule_df[['shortChannelId_incoming', 'shortChannelId_outgoing']].values.ravel('K')
    )
    channel_results = {}
    for channel in unique_channels:
        result_df = was_channel_jammed(channel, pair_schedule_df)
        channel_results[channel] = result_df
    return channel_results

def find_channels_with_high_slots(all_channel_results, threshold=50):
    channels_with_high_slots = []
    for channel, df in all_channel_results.items():
        if (df['taken_slots'] > threshold).any():
            channels_with_high_slots.append(channel)
    return channels_with_high_slots

def calculate_cumulative_time(chan_df, from_slots):
    cumulative_time_ns = 0
    start_time = None

    for i in range(len(chan_df)):
        if chan_df.iloc[i]['taken_slots'] >= from_slots and start_time is None:
            start_time = chan_df.iloc[i]['time']
        elif chan_df.iloc[i]['taken_slots'] < from_slots and start_time is not None:
            cumulative_time_ns += chan_df.iloc[i]['time'] - start_time
            start_time = None

    # Convert time from nanoseconds to minutes
    cumulative_time_minutes = cumulative_time_ns / (1e9 * 60)
    return cumulative_time_minutes

def create_pair_schedule_df(json_file_path):
    with open(json_file_path, 'r') as file:
        data = json.load(file)
    
    
    forwards = data.get('forwards', [])
    
    # Extract relevant fields
    rows = []
    for entry in forwards:
        if isinstance(entry, dict):
            rows.append({
                'eventTimeNs': int(entry['addTimeNs']),
                'eventType': 'add',
                'shortChannelId_outgoing': int(entry['outgoingCircuit']['shortChannelId']),
                'shortChannelId_incoming': int(entry['incomingCircuit']['shortChannelId']),
            })
            if 'resolveTimeNs' in entry:
                rows.append({
                    'eventTimeNs': int(entry['resolveTimeNs']),
                    'eventType': 'resolve',
                    'shortChannelId_outgoing': int(entry['outgoingCircuit']['shortChannelId']),
                    'shortChannelId_incoming': int(entry['incomingCircuit']['shortChannelId']),
                })
        else:
            print(f"Expected a dictionary but got {type(entry)}.")
    
    # Create DataFrame
    pair_schedule_df = pd.DataFrame(rows)
    pair_schedule_df = pair_schedule_df.sort_values(by='eventTimeNs').reset_index(drop=True)
    return pair_schedule_df


def get_jam_time(forwarding_history_file, lower_bound):
    pair_schedule_df = create_pair_schedule_df(forwarding_history_file)

    if pair_schedule_df is not None:
        all_channel_results = process_all_channels(pair_schedule_df)
        channels_with_high_slots = find_channels_with_high_slots(all_channel_results, lower_bound)

        # Assuming there is only one channel with high slots, get the first one
        if channels_with_high_slots:
            selected_channel = channels_with_high_slots[0]
            chan_df = was_channel_jammed(selected_channel, pair_schedule_df)
        
            return calculate_cumulative_time(chan_df, lower_bound)
    else:
        print("Failed to create pair schedule DataFrame.")

if __name__ == '__main__':
    if len(sys.argv) != 2:
        print("Usage: python script.py forwarding_history_file")
        sys.exit(1)

    forwarding_history_file = sys.argv[1]

    lower_bound = 440

    jam_time = get_jam_time(forwarding_history_file, lower_bound)
    print(f"Cumulative jam time occupied > 440 slots: {jam_time}")

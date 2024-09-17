import json
import sys
import matplotlib.pyplot as plt
import datetime
import os

def process_htlcs(file_path, skip_scid):
    channel_data = {}

    with open(file_path, 'r') as file:
        try:
            data = json.load(file)
            htlcs = data.get('htlcs', [])
            
            for htlc in htlcs:
                outgoing_channel = htlc.get('outgoingChannel')
                if outgoing_channel == skip_scid:
                    continue

                # Initialize data for the outgoing channel if not already present
                if outgoing_channel not in channel_data:
                    channel_data[outgoing_channel] = ([], [])

                timestamp_ns = int(htlc.get('forwardTsNs', 0))
                outcome = int(htlc.get('outcome', 0))

                # Add the timestamp for this forward and its outcome
                timestamp = datetime.datetime.fromtimestamp(timestamp_ns / 1e9)
                timestamps, outcomes = channel_data[outgoing_channel]

                timestamps.append(timestamp)

                # Map outcomes for better visual representation
                if outcome == 2:
                    outcomes.append(0)
                elif outcome == 1:
                    outcomes.append(1)
                elif outcome == 0:
                    outcomes.append(2)
                elif outcome == 3:
                    outcomes.append(3)

        except json.JSONDecodeError as e:
            print(f"Error reading {file_path}: {e}")

    return channel_data

def save_graphs(channel_data):
    for channel_id, (timestamps, outcomes) in channel_data.items():
        plt.figure(figsize=(10, 5))
        plt.plot(timestamps, outcomes, marker='s', label="Outcome", color='green', linestyle='--')

        # Add titles and labels
        plt.title(f"Forward Decisions for Channel {channel_id}")
        plt.xlabel("Timestamp")
        plt.ylabel("Outcome")

        # Rotate the x-axis labels for better readability
        plt.xticks(rotation=45)

        plt.yticks([0, 1, 2, 3], [
            "Endorsed Forward",
            "Unendorsed Forward",
            "No Resource Dropped",
            "Low Reputation Dropped"
        ])

        plt.tight_layout()

        # Save the figure with the channel_id as its filename
        plt.savefig(f"{channel_id}.png")
        plt.close()

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python channel_reputation.py target_threshold_path short_channel_id")
        sys.exit(1)

    target_threshold = sys.argv[1]
    skip_scid = sys.argv[2]

    channel_data = process_htlcs(target_threshold, skip_scid)
    save_graphs(channel_data)

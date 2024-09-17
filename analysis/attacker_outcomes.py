import json
import sys
import matplotlib.pyplot as plt
import datetime

def channel_reputation(file_path, scid):
    timestamps = []
    outcomes = []

    with open(file_path, 'r') as file:
        try:
            data = json.load(file)
            htlcs = data.get('htlcs', [])
            
            for htlc in htlcs:
                if htlc.get('outgoingChannel') == scid:
                    timestamp_ns = int(htlc.get('forwardTsNs', 0))
                    outcome = int(htlc.get('outcome', 0))

                    # Add the timestamp for this forward and its outcome
                    timestamp = datetime.datetime.fromtimestamp(timestamp_ns / 1e9)
                    timestamps.append(timestamp)

                    # The values that we use for outcomes don't map nicely to a visual
                    # representation (eg, values 0 and 3 both represent a dropped value).
                    # To be able to graph them nicely, we re-assign values.
                    # forward endorsed 2 -> 0
                    # forward unendorsed 1 -> 1
                    # drop no resources 0 -> 2
                    # drop low rep 3 -> 3
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

    return timestamps, outcomes

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python attacker_outcomes.py target_threshold_path short_channel_id")
        sys.exit(1)

    target_threshold = sys.argv[1]
    scid = sys.argv[2]

    timestamps, outcomes = channel_reputation(target_threshold, scid)

    plt.figure(figsize=(10, 5))
    plt.plot(timestamps, outcomes, marker='s', label="Outcome", color='green', linestyle='--')

    # Add titles and labels
    plt.title("Forwards: Target -> Attacker")
    plt.xlabel("Timestamp")

    # Rotate the x-axis labels for better readability
    plt.xticks(rotation=45)

    plt.yticks([0, 1, 2, 3], [
        "Endorsed Forward",
        "Unendorsed Forward",
        "No Resource Dropped",
        "Low Reputation Dropped"
    ])

    plt.tight_layout()
    plt.show()

import json
import glob
import sys

# Define the directory path where your JSON files are located
directory_path=sys.argv[1]

# Find all files matching the pattern '*thresholds.json'
file_paths = glob.glob(f"{directory_path}/*thresholds.json")

total_htlcs = 0
total_outcome_0 = 0
total_outcome_1 = 0
total_outcome_2 = 0
total_outcome_3 = 0

# Iterate over each file
for file_path in file_paths:
    with open(file_path, 'r') as file:
        try:
            # Load the JSON data
            data = json.load(file)
            # Extract the 'htlcs' list
            htlcs = data.get('htlcs', [])
            # Update the total number of HTLCs
            total_htlcs += len(htlcs)
            # Count the number of HTLCs with outcome = 2
            total_outcome_0 += sum(1 for htlc in htlcs if htlc.get('outcome') == 0)
            total_outcome_1 += sum(1 for htlc in htlcs if htlc.get('outcome') == 1)
            total_outcome_2 += sum(1 for htlc in htlcs if htlc.get('outcome') == 2)
            total_outcome_3 += sum(1 for htlc in htlcs if htlc.get('outcome') == 3)
        except json.JSONDecodeError as e:
            print(f"Error reading {file_path}: {e}")

# Print the results
print(f"Total HTLCs: {total_htlcs}")
print(f"Total HTLCs dropped no resources: {total_outcome_0}")
print(f"Total HTLCs forwarded unendorsed: {total_outcome_1}")
print(f"Total HTLCs forwarded endorsed: {total_outcome_2}")
print(f"Total HTLCs dropped due to unknown outgoing: {total_outcome_3}")

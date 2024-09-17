import sys
import time
from datetime import datetime
from pathlib import Path

def progress_timestamps(csv_file: Path, outfile: Path):
    with open(csv_file, 'r') as f:
        headers = next(f).strip().split(',')  # Read and skip the headers

        # Read the rest of the lines and process them
        csv_data = [line.strip().split(',') for line in f]
    
    # Find the difference between the current time and the latest timestamp in the CSV and progress timestamps by that amount.
    time_difference = get_time_difference(csv_data)
    progress_timestamps_helper(csv_data, time_difference)

    # Write updated CSV data
    write_csv_data(headers, csv_data, outfile)
    print("CSV data updated and written to", outfile)

def find_latest_timestamp(csv_data):
    latest_timestamp = None
    timestamp_indices = [2, 3, 6, 7]  # Indices of timestamp fields
    for line in csv_data:
        for index in timestamp_indices:
            timestamp = int(line[index])
            if latest_timestamp is None or timestamp > latest_timestamp:
                latest_timestamp = timestamp
    return latest_timestamp

def progress_timestamps_helper(csv_data, time_difference):
    timestamp_indices = [2, 3, 6, 7]  # Indices of timestamp fields
    current_time_ns = int(time.time() * 1e9)

    for line in csv_data:
        for index in timestamp_indices:
            timestamp = int(line[index])
            updated_timestamp = timestamp + time_difference
            updated_timestamp = max(updated_timestamp, current_time_ns)
            line[index] = str(updated_timestamp)

def get_time_difference(csv_data):
    latest_timestamp = find_latest_timestamp(csv_data)
    if latest_timestamp is None:
        print("No timestamps found in CSV file.")
        exit(1)

    # Calculate time difference
    current_time_ns = int(time.time() * 1e9)
    time_difference = current_time_ns - latest_timestamp
    return time_difference

def write_csv_data(headers, csv_data, outfile):
    with open(outfile, 'w') as f:
        f.write(','.join(headers) + '\n')  # Write headers
        for line in csv_data:
            f.write(','.join(line) + '\n')

if __name__ == "__main__":
    if len(sys.argv) < 2 or len(sys.argv) > 3:
        print("Usage: python program.py <csv_file> [<output_file>]")
        sys.exit(1)

    csv_file_path = Path(sys.argv[1])
    if not csv_file_path.exists():
        print("Error: CSV file not found.")
        sys.exit(1)

    if len(sys.argv) == 3:
        outfile_path = Path(sys.argv[2])
    else:
        outfile_path = csv_file_path.stem + "_timewarp.csv"
    
    progress_timestamps(csv_file_path, outfile_path)

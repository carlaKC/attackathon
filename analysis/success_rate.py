import os
import sys
import json

def calculate_success_rate(directory):
    # Initialize counters
    succeeded_count = 0
    failed_count = 0

    for root, _, files in os.walk(directory):
        for file in files:
            if file.endswith("payments.json"):
                file_path = os.path.join(root, file)
                with open(file_path, 'r') as f:
                    try:
                        data = json.load(f)
                    except json.JSONDecodeError:
                        print(f"Failed to parse {file_path}. Skipping.")
                        continue

                    # Iterate over each payment in the current file and count statuses
                    for payment in data.get("payments", []):
                        status = payment.get("status")
                        if status == "SUCCEEDED":
                            succeeded_count += 1
                        elif status == "FAILED":
                            failed_count += 1

    total_count = succeeded_count + failed_count

    if total_count == 0:
        print("No payments found across files.")
        return

    # Calculate success rate
    success_rate = (succeeded_count / total_count) * 100

    # Output results
    print(f"Total payments across all files: {total_count}")
    print(f"Succeeded: {succeeded_count}")
    print(f"Failed: {failed_count}")
    print(f"Success rate: {success_rate:.2f}%")

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python success_rate.py <path_to_payments_file>")
        sys.exit(1)

    directory_path = sys.argv[1]
    calculate_success_rate(directory_path)

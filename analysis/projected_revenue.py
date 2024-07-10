import statistics
import sys
import csv
import time
import os
import target_jammed

def get_projected_revenue(file_path, node_id, revenue_period_ns):
    total_fees = 0
    timestamp_limit = None

    with open(file_path, 'r') as file:
        reader = csv.DictReader(file)
        for row in reader:
            incoming_amt = int(row['incoming_amt'])
            outgoing_amt = int(row['outgoing_amt'])
            forwarding_alias = row['forwarding_alias']
            incoming_add_ts = int(row['incoming_add_ts'])
            incoming_remove_ts = int(row['incoming_remove_ts'])
           
            # We want to only get entries for the period that we've defined to have a way to compare revenue to what we got in the simulation 
            # that ran for revenue_period_ns. We don't have a start time for this projected data, so we just grab our first timestamp as the 
            # start. This is imperfect, and may lead to us over-estimating revenue without an attack (especially if there was a long wait 
            # for the first payment to occur). This could possibly be improved by including the start time in the file name so we can get 
            # an exact start, but is okay for now.
            # 
            # We can't use actual timestamps here, because this data was generated once-off and has old timestamps (hasn't been "progressed"
            # to current times like we do for bootstrapped data, as this isn't actually necessary).
            if timestamp_limit is None:
                timestamp_limit = incoming_add_ts + revenue_period_ns

            if incoming_add_ts >= timestamp_limit:
                break
            
            if forwarding_alias == node_id and incoming_remove_ts < timestamp_limit:
                total_fees += (incoming_amt - outgoing_amt)
    
    return total_fees


def get_revenue_stats(network_name, node_id, revenue_period_ns):
    data_path = os.path.join("data", network_name, "projections")
    data_path = os.path.join("data", network_name, "projections")
    if not os.path.exists(data_path):
        print(f"Projected revenue not generated in {data_path}. Please run `./attackathon/setup/get_projections.sh` {network_name} to generate it.")
        sys.exit(1)

    projected_values = []
    for filename in os.listdir(data_path):
        projected_data = os.path.join(data_path, filename)
        projected = get_projected_revenue(projected_data, node_id, revenue_period_ns)
        projected_values.append(projected)

    mean_revenue = statistics.mean(projected_values)
    std_dev_revenue = statistics.stdev(projected_values)

    return mean_revenue, std_dev_revenue
    
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python projected_revenue.py <network_name>") 
        sys.exit(1)

    network_name = sys.argv[1]
    network_path = os.path.join("data", network_name)

    target_path = os.path.join(network_path, "target.txt")
    if os.path.exists(target_path):
        with open(target_path, 'r') as file:
            node_id = file.read().strip()
            print(f"Running attack analysis for target node: {node_id}")
    else:
        print(f"The network at {target_path} does not exist.")
        sys.exit(1)

    mean_revenue, std_dev = get_revenue_stats(network_name, node_id, time.time_ns())

    print(f"Mean target revenue without attack: {mean_revenue} msat")
    print(f"- Standard deviation: {std_dev} msat")
    print(f"- Mean/Std Dev: {mean_revenue/std_dev}")

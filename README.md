# Attackathon

![image info](hackerman.jpg)

## Task 

In this attackathon, your task will be to write a program that performs
a [channel jamming attack](https://bitcoinops.org/en/topics/channel-jamming-attacks/) 
against a test lightning network. Your goal is to **completely jam a 
routing node for an hour.**

Your program should: 
- Accept the public key of the node being attacked as a parameter. 
- Write an attack against the [hybrid approach to jamming mitigations](https://research.chaincode.com/2022/11/15/unjamming-lightning/)
  which is deployed on the network.
- Open any channels required to perform the attack, and close them 
  when the attack has competed.

The final deliverable for the attackathon is a [run script](./setup/run.sh) 
that downloads, installs and runs your attack in a kubernetes cluster.

## Jamming Definition

We're aiming to jam a routing node, which we define as: 
```
The target node is unable to facillitate forwading of HTLCs on behalf 
of of nodes in the network.
```

Conventionally, this is achieved when:
```
All of the target node's local(/outbound) HTLC slots are occupied.
OR 
All of the target node's local(/outbound) liquidity is occupied.
```

However, given that we are operating within the context of a specific 
mitigation, we need to consider the possibility that the attack may 
try to use the mitigation _itself_ to disrupt quality of service.

In general, our reputation and resource bucketing mitigation may be 
abused by an attacker to jam a channel if: 
```
All of its general bucket's local(/outbound) liquidity OR slots are occupied.
AND
Peers looking to use the channel have low reputation.
```

When the attacker manages to successfully sabotage the reputation and 
fill up **general slots**, the channel is effectively jammed because 
peers looking to use the channel do not have access to the 
**projected slots** that are reserved for high reputation peers. This 
may be abused in various ways, and we encourage you to explore them!

Note: There is a special case of channel jamming for trimmed HTLCs.
If the node implementation supports `max_dust_htlc_exposure_msat`, this
introduces an absolute limit in satoshis on the number of trimmed HTLCs
a target node can forward concurrently.

### Development Environment

The attack you develop will be tested against a [warnet](https://warnet.dev/)
running a network of [LND](https://github.com/carlaKC/lnd/tree/attackathon) 
nodes that have the jamming attack mitigation implemented* (via an 
external tool called circuitbreaker).

To assist with local development, we've provided a test network that 
can be used to run your attacks against. Prerequisites to set up this 
network are: 
* Python3
* Docker
* Kubernetes (see [docker desktop](https://docs.docker.com/desktop/kubernetes/) or [minikube](https://minikube.sigs.k8s.io/docs/start/) instructions)
* [Just](https://github.com/casey/just)
* [jq](https://jqlang.github.io/jq)

Clone the attackathon repo:
`git clone https://github.com/carlaKC/attackathon`

*Do not change directory.*

The scripts will pull the relevant repositories to your current working
directory and set up your network. They expect the `attackathon` 
repository to be in the current directory.
* Warnet server: [./attackathon/scripts/start_warnet.sh](./scripts/start_warnet.sh) 
  sets up the warnet server, which is responsible for orchestration of 
  the network. 
  * You'll only need to do this once, but leave it running!
  * When you're done with it, bring it down with 
    [./attackathon/scripts/stop_warnet.sh](./scripts/stop_warnet.sh)
* Start network: [./attackathon/scripts/start_network.sh ln_10](/.scripts/start_network.sh)
  brings up your lightning network, opens channels, simulates 
  random payments in the network and mines blocks every 5 minutes.
  * If you want to kill your test network and start fresh, you can 
    use [./attackathon/scripts/stop_network.sh ln_10](./scripts/stop_network.sh)
  * You need to wait for this script to complete before you can start 
    your attacking pod!

Wait for your network to fully come up, then you can start your pod 
of attacking nodes:
* Start attacking pods: [./attackathon/scripts/start_attacker.sh ln_10](./scripts/start_attacker.sh)
  brings up three lightning nodes that you will use for your attack, 
  a bitcoin node and an empty `flagship` container to run your attack 
  from.
  * You can use [./attackathon/scripts/stop_attacker.sh](./scripts/stop_attacker.sh) 
    to tear this down if you'd like to start over at any point.

Once you have brought your cluster up, you'll be able to execute your 
program from *inside* of the cluster's `flagship` pod:
* `kubectl exec -it flagship -n warnet-armada -- bash`
* Update `run.sh` to:
  * Install your program.
  * Run your program using the credentials provided inline.

⚠️ Remember that we're using a [fork](https://github.com/carlaKC/lnd/tree/attackathon)
of LND so you'll need to account for that if you're using common RPC 
client libraires to have access to `endorsement` fields (golang client 
is available [here](https://github.com/carlaKC/lndclient/tree/attackathon)).

The following utilities are available for your convenience:
* `source ./lncli.sh` provides aliases for your LND nodes (`lncli0`, 
  `lncli1`, `lncli2`)
* `./fund.sh` funds each of your LND nodes.
* `./connect_nodes.sh` connects the attacking nodes to the network 
   so that they can sync gossip.
* `bitcoin-cli` provides access to the bitcoin node that all three 
  LND nodes are connected to.

## Network Information

Some relevant characteristics of the network: 
- The reputation system has been primed with historical forwarding 
  data, so nodes in the network have already had a chance to build 
  up reputation before the attack begins.
- Each node in the network: 
  - Allows 483 HTLCs in flight per channel.
  - Allows 45% of its channel's capacity in flight.
  - Allocates 50% of these resources to "general" traffic, and 50% to 
    protected traffic.
- The graph was obtained by reducing the mainnet graph using a 
  random walk around our target node, and real-world routing policies 
  are used.
- When you run the attack, the non-malicious nodes in the network will 
  be executing [randomly generated payments](https://simln.dev) to 
  mimic an active network.

Some APIS to note:
- [AddHoldInvoice](https://lightning.engineering/api-docs/api/lnd/invoices/add-hold-invoice)
  creates an invoice that can be manually [settled](https://lightning.engineering/api-docs/api/lnd/invoices/settle-invoice) 
  or [canceled](https://lightning.engineering/api-docs/api/lnd/invoices/cancel-invoice)
- Endorsement signals can be set on the [SendToRoute](https://lightning.engineering/api-docs/api/lnd/router/send-to-route-v2)
  or [SendPayment](https://lightning.engineering/api-docs/api/lnd/router/send-payment-v2)
  APIs.

\* Note that endorsement signaling and reputation tracking are fully 
deployed on the test network, but unconditional fees are not. You should
assume that they will be 1% of your success-case fees, and we will 
account for them during attack analysis.

## Assessment

Attacks will be assessed using the following measures:
- Did the attack achieve a jamming attack as described above?
- What was the total cost of the attack, considering:
  - On-chain fees: for channel opens and closes, sending funds between 
    nodes on-chain will node be included for simplicity's sake.
  - Off-chain fees: the sum of fees paid for successful off-chain 
    payments plus 1% of the success-case fees for *all* payments that 
    are sent to represent unconditional fees.
  - Opportunity cost of capital: for each channel that is opened, 5% 
    p.a. charged on the total capital deployed in the channels, 
    assuming 10 minute blocks.
- When compared to the operation of the network _without_ a jamming 
  attack, how many honest htlcs were dropped as a result of the attack?

A work-in-progress analysis script can be run using:
`python3 analysis/analyse_attack.py`

### HackNicePlz

We're trying to break channel jamming mitigations, not our setup itself
so please be a good sport and let us know if there's anything buggy! 
Real attackers won't be able to take advantage of our test setup, so 
neither should we.


## Network Creation

Participants do not need to read the following section, it contains 
instructions on how to setup a warnet network to run the attackathon 
on.

<details>
 <summary>Setup Instructions</summary>

To get started, you will need to clone the following repos *in the same
working directory*:
1. [This repo](https://github.com/carlaKC/attackathon)
2. [Warnet](https://github.com/bitcoin-dev-project/warnet)
3. [SimLN](https://github.com/bitcoin-dev-project/sim-ln)
3. [Circuitbreaker](https://github.com/lightningequipment/circuitbreaker)

You will need to provide: 
1. A `json` file with the same format as LND's `describegraph` output 
  which describes the graph that you'd like to simulate.
2. The duration of time, expressed in seconds, that you'd like the 
  setup script to generate fake historical forwards for all the nodes 
  in the network for.
3. Manually add the alias of the node that you're attacking to 
  `attackathon/data/{network name}/target.txt` once this script has 
  run.

The setup script provided will generate all required files and docker 
images for you:
`./attackathon/setup/create_network.sh {path to json file} {duration in seconds}`

Note that you *must* run this from your directory containing `warnet`, 
`simln` and `circuitbreaker` because it moves between directories to 
achieve various tasks! The name that you give the `json` file is 
considered to be your `network_name`. 

Once the script has completed, check in any files that it generated and 
provide your students with the following: 
1. The `network_name` for your attackathon.
2. The attackathon repo (/branch) with all files checked in.

The script currently hardcodes the docker hub account for images to 
`carlakirkcohen` and tries to push to it, so you'll need to search and 
replace if you want to test the full flow.

</details>

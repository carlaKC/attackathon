# Use Ubuntu 24.04 as base image
FROM ubuntu:24.04

# Install required utilities
RUN apt-get update && apt-get install -y wget vim curl jq git make

# Download and install neovim
RUN apt-get install -y neovim

# Install Golang
RUN apt-get install -y golang

# Install rust + set cargo path
RUN curl https://sh.rustup.rs -sSf | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# Create bitcoin conf so that bitcoin-cli just works.
RUN mkdir -p ~/.bitcoin && \
    echo "rpcconnect=bitcoind.warnet-armada" >> ~/.bitcoin/bitcoin.conf && \
    echo "rpcuser=warnet_user" >> ~/.bitcoin/bitcoin.conf && \
    echo "rpcpassword=2themoon" >> ~/.bitcoin/bitcoin.conf && \
    echo "regtest=1" >> ~/.bitcoin/bitcoin.conf

# Install bitcoind/lnd clis.
COPY install_cli.sh /install_cli.sh
RUN chmod +x install_cli.sh
RUN ./install_cli.sh

# Copy in run script that is a placeholder for attacker code.
COPY run.sh /run.sh
RUN chmod +x run.sh

COPY lncli.sh /lncli.sh

COPY fund.sh /fund.sh
RUN chmod +x fund.sh

COPY connect_nodes.sh /connect_nodes.sh
RUN chmod +x connect_nodes.sh

# Set entrypoint to just sleep indefinitely - attackers will run their own scripts.
ENTRYPOINT ["bash", "-c", "while true; do sleep 30; done;"]

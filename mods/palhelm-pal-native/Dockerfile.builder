FROM gcc:13

RUN apt-get update \
    && apt-get install -y --no-install-recommends clang libc++-dev libc++abi-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /source

ENTRYPOINT ["clang++"]
